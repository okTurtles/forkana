// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	repo_service "code.gitea.io/gitea/services/repository"
)

func acceptTransfer(ctx *context.Context) {
	repo := ctx.Repo.Repository
	// An archived article is read-only for its owner, so a transfer started before it
	// was archived must not be completed.
	if repo.SubjectID > 0 && repo.IsArchived {
		archivedAt := util.Iif(repo.ArchivedUnix.IsZero(), repo.UpdatedUnix, repo.ArchivedUnix)
		ctx.Flash.Error(ctx.Tr("repo.settings.article_archived_notice", archivedAt.FormatDate()))
		ctx.Redirect(repo.Link())
		return
	}

	err := repo_service.AcceptTransferOwnership(ctx, ctx.Repo.Repository, ctx.Doer)
	if err == nil {
		ctx.Flash.Success(ctx.Tr("repo.settings.transfer.success"))
		ctx.Redirect(ctx.Repo.Repository.Link())
		return
	}
	handleActionError(ctx, err)
}

func rejectTransfer(ctx *context.Context) {
	err := repo_service.RejectRepositoryTransfer(ctx, ctx.Repo.Repository, ctx.Doer)
	if err == nil {
		ctx.Flash.Success(ctx.Tr("repo.settings.transfer.rejected"))
		ctx.Redirect(ctx.Repo.Repository.Link())
		return
	}
	handleActionError(ctx, err)
}

func ActionTransfer(ctx *context.Context) {
	switch ctx.PathParam("action") {
	case "accept_transfer":
		acceptTransfer(ctx)
	case "reject_transfer":
		rejectTransfer(ctx)
	}
}
