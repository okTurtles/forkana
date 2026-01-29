// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"net/http"
	"slices"

	auth_model "code.gitea.io/gitea/models/auth"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
)

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
// If the request has fork_and_edit=true or submit_change_request=true in the form data,
// the check is skipped because the handler will create a fork/branch and commit to that instead.
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

		// Allow fork-and-edit workflow to bypass write permission check for _edit and _new
		// The handler will create a personal fork and commit to that instead
		if ctx.Req.FormValue("fork_and_edit") == "true" {
			if editorAction == "_edit" || editorAction == "_new" {
				return
			}
		}

		// Allow submit-change-request workflow to bypass write permission check for _edit only
		// This workflow creates an in-repo branch and PR to propose changes to existing articles
		// It does NOT support _new - creating new files should be done in the user's own repository
		if ctx.Req.FormValue("submit_change_request") == "true" {
			if editorAction == "_edit" {
				return
			}
			// For _new action with submit_change_request, fall through to permission check
			// which will correctly deny access for non-collaborators
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
	if ok { // it's a personal access token but not oauth2 token
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
