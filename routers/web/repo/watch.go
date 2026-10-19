// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/templates"
	"code.gitea.io/gitea/services/context"
)

const (
	tplWatchUnwatch templates.TplName = "repo/watch_unwatch"
	// tplWatchButton is the Article view's Watch button (#433, figma 641:65480), which
	// posts with style=button so that it is replaced by itself rather than by the
	// subject header's Follow star.
	tplWatchButton templates.TplName = "repo/watch_button"
)

func ActionWatch(ctx *context.Context) {
	err := repo_model.WatchRepo(ctx, ctx.Doer, ctx.Repo.Repository, ctx.PathParam("action") == "watch")
	if err != nil {
		handleActionError(ctx, err)
		return
	}

	ctx.Data["IsWatchingRepo"] = repo_model.IsWatching(ctx, ctx.Doer.ID, ctx.Repo.Repository.ID)
	ctx.Data["Repository"], err = repo_model.GetRepositoryByName(ctx, ctx.Repo.Repository.OwnerID, ctx.Repo.Repository.Name)
	if err != nil {
		ctx.ServerError("GetRepositoryByName", err)
		return
	}
	ctx.RespHeader().Add("hx-trigger", "refreshUserCards") // see the `hx-trigger="refreshUserCards ..."` comments in tmpl
	if ctx.FormString("style") == "button" {
		ctx.HTML(http.StatusOK, tplWatchButton)
		return
	}
	ctx.HTML(http.StatusOK, tplWatchUnwatch)
}
