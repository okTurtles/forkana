// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"net/http"
	"slices"

	auth_model "code.gitea.io/gitea/models/auth"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/web"
)

// editorWorkflowForm mirrors forms.EditorWorkflowForm, which is implemented by the
// bound editor forms. It is redeclared here because services/forms imports this
// package, so this package cannot import it back.
type editorWorkflowForm interface {
	IsForkAndEdit() bool
	IsSubmitChangeRequest() bool
}

// RequireRepoAdmin returns a middleware for requiring repository admin permission
func RequireRepoAdmin() func(ctx *Context) {
	return func(ctx *Context) {
		if !ctx.IsSigned || !ctx.Repo.IsAdmin() {
			ctx.NotFound(nil)
			return
		}
	}
}

// CanWriteToBranch checks if the user is allowed to write to the branch of the repo
// If the request has a truthy fork_and_edit or submit_change_request in the form data,
// the check is skipped because the handler will create a fork/branch and commit to that instead.
// Both flags are read from the form bound by web.Bind, which runs before this middleware on
// the editor routes, so that this gate and the editor handler always agree on whether the
// workflow is active: a value the binder rejects is bound to false and is therefore treated
// as false here as well. Requests without a bound editor form fall through to the
// permission check.
//
// Workflow support by action:
//   - fork_and_edit: supports both _edit and _new (creates a personal fork)
//   - submit_change_request: supports only _edit (proposes changes to existing articles via in-repo PR)
//
// The submit_change_request workflow intentionally does NOT support _new because creating new files
// in someone else's repository doesn't align with the Forkana model - users should create their own
// repository for new articles rather than proposing to add files to another user's repository.
//
// Other actions (delete, upload, diffpatch, cherrypick) do NOT support these workflows
// and must not allow this bypass.
func CanWriteToBranch() func(ctx *Context) {
	return func(ctx *Context) {
		editorAction := ctx.PathParam("editor_action")

		if editorForm, ok := web.GetForm(ctx).(editorWorkflowForm); ok {
			// Allow fork-and-edit workflow to bypass write permission check for _edit and _new
			// The handler will create a personal fork and commit to that instead
			if editorForm.IsForkAndEdit() && (editorAction == "_edit" || editorAction == "_new") {
				return
			}

			// Allow submit-change-request workflow to bypass write permission check for _edit only
			// This workflow creates an in-repo branch and PR to propose changes to existing articles
			// It does NOT support _new - creating new files should be done in the user's own repository
			// For _new action with submit_change_request, fall through to permission check
			// which will correctly deny access for non-collaborators
			if editorForm.IsSubmitChangeRequest() && editorAction == "_edit" {
				return
			}
		}
		if !ctx.Repo.CanWriteToBranch(ctx, ctx.Doer, ctx.Repo.BranchName) {
			ctx.NotFound(nil)
			return
		}
	}
}

// RequireUnitWriter returns a middleware for requiring repository write to one of the unit permission
func RequireUnitWriter(unitTypes ...unit.Type) func(ctx *Context) {
	return func(ctx *Context) {
		if slices.ContainsFunc(unitTypes, ctx.Repo.CanWrite) {
			return
		}
		ctx.NotFound(nil)
	}
}

// RequireUnitReader returns a middleware for requiring repository write to one of the unit permission
func RequireUnitReader(unitTypes ...unit.Type) func(ctx *Context) {
	return func(ctx *Context) {
		for _, unitType := range unitTypes {
			if ctx.Repo.CanRead(unitType) {
				return
			}
			if unitType == unit.TypeCode && canWriteAsMaintainer(ctx) {
				return
			}
		}
		ctx.NotFound(nil)
	}
}

// CheckRepoScopedToken check whether personal access token has repo scope
func CheckRepoScopedToken(ctx *Context, repo *repo_model.Repository, level auth_model.AccessTokenScopeLevel) {
	if !ctx.IsBasicAuth || ctx.Data["IsApiToken"] != true {
		return
	}

	scope, ok := ctx.Data["ApiTokenScope"].(auth_model.AccessTokenScope)
	if ok { // it's an API token (personal access token or OAuth2 token) whose scope must be enforced
		var scopeMatched bool

		requiredScopes := auth_model.GetRequiredScopes(level, auth_model.AccessTokenScopeCategoryRepository)

		// check if scope only applies to public resources
		publicOnly, err := scope.PublicOnly()
		if err != nil {
			ctx.ServerError("HasScope", err)
			return
		}

		if publicOnly && repo.IsPrivate {
			ctx.HTTPError(http.StatusForbidden)
			return
		}

		scopeMatched, err = scope.HasScope(requiredScopes...)
		if err != nil {
			ctx.ServerError("HasScope", err)
			return
		}

		if !scopeMatched {
			ctx.HTTPError(http.StatusForbidden)
			return
		}
	}
}
