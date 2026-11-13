// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package moderation serves the moderation workspace (/-/moderation). Its pages are
// placeholders until the queue (1.5) and the case review (1.6) land.
package moderation

import (
	"net/http"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
	moderation_service "code.gitea.io/gitea/services/moderation"
)

const (
	tplQueue templates.TplName = "moderation/queue"
	tplCase  templates.TplName = "moderation/case"
)

// RequireEnabled answers 404 while moderation is off, before anything else (signing in
// included) is checked.
func RequireEnabled(ctx *context.Context) {
	if !setting.Moderation.Enabled {
		ctx.NotFound(nil)
	}
}

// RequireModerator lets only moderators through. Everyone else gets a 404, so the workspace's
// existence isn't revealed. Signing in is checked before, by the route's reqSignIn.
func RequireModerator(ctx *context.Context) {
	ok, err := moderation_service.IsModerator(ctx, ctx.Doer)
	if err != nil {
		ctx.ServerError("IsModerator", err)
		return
	}
	if !ok {
		ctx.NotFound(nil)
		return
	}
	ctx.Data["PageIsModeration"] = true
}

// Queue renders the moderation queue (placeholder).
func Queue(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("moderation.title")
	ctx.HTML(http.StatusOK, tplQueue)
}

// Case renders one case (placeholder). It answers 404 for an index that isn't a case of the
// cases repository.
func Case(ctx *context.Context) {
	repo, err := moderation_service.GetCasesRepo(ctx)
	if err != nil {
		ctx.NotFoundOrServerError("GetCasesRepo", repo_model.IsErrRepoNotExist, err)
		return
	}
	issue, err := issues_model.GetIssueByIndex(ctx, repo.ID, ctx.PathParamInt64("index"))
	if err != nil {
		ctx.NotFoundOrServerError("GetIssueByIndex", issues_model.IsErrIssueNotExist, err)
		return
	}
	ctx.Data["Title"] = ctx.Tr("moderation.case.title", issue.Index)
	ctx.Data["CaseIndex"] = issue.Index
	ctx.HTML(http.StatusOK, tplCase)
}
