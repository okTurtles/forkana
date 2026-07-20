// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"fmt"
	"net/http"

	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/httpcache"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/routers/common"
	"code.gitea.io/gitea/services/attachment"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/context/upload"
	repo_service "code.gitea.io/gitea/services/repository"
)

// UploadIssueAttachment response for Issue/PR attachments
func UploadIssueAttachment(ctx *context.Context) {
	uploadAttachment(ctx, ctx.Repo.Repository.ID, setting.Attachment.AllowedTypes)
}

// UploadReleaseAttachment response for uploading release attachments
func UploadReleaseAttachment(ctx *context.Context) {
	uploadAttachment(ctx, ctx.Repo.Repository.ID, setting.Repository.Release.AllowedTypes)
}

// UploadEditorAttachment uploads an attachment for the file/article editor. Unlike issue or
// release attachments, these are never linked to an issue/release: they are referenced only
// from the committed Markdown by RepoID. ServeAttachment authorizes such attachments by
// repository read permission so embedded article images are visible to readers.
func UploadEditorAttachment(ctx *context.Context) {
	uploadAttachment(ctx, ctx.Repo.Repository.ID, setting.Attachment.AllowedTypes)
}

// UploadAttachment response for uploading attachments
func uploadAttachment(ctx *context.Context, repoID int64, allowedTypes string) {
	if !setting.Attachment.Enabled {
		ctx.HTTPError(http.StatusNotFound, "attachment is not enabled")
		return
	}

	file, header, err := ctx.Req.FormFile("file")
	if err != nil {
		if isUploadSizeError(err) {
			ctx.HTTPError(http.StatusRequestEntityTooLarge, uploadTooLargeMessage())
			return
		}
		ctx.HTTPError(http.StatusInternalServerError, fmt.Sprintf("FormFile: %v", err))
		return
	}
	defer file.Close()

	attach, err := attachment.UploadAttachment(ctx, file, allowedTypes, header.Size, &repo_model.Attachment{
		Name:       header.Filename,
		UploaderID: ctx.Doer.ID,
		RepoID:     repoID,
	})
	if err != nil {
		if upload.IsErrFileTypeForbidden(err) {
			ctx.HTTPError(http.StatusBadRequest, err.Error())
			return
		}
		if upload.IsErrFileTooLarge(err) {
			ctx.HTTPError(http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		ctx.HTTPError(http.StatusInternalServerError, fmt.Sprintf("NewAttachment: %v", err))
		return
	}

	log.Trace("New attachment uploaded: %s", attach.UUID)
	ctx.JSON(http.StatusOK, map[string]string{
		"uuid": attach.UUID,
		"url":  setting.AppSubURL + "/attachments/" + attach.UUID,
	})
}

// DeleteAttachment response for deleting issue's attachment
func DeleteAttachment(ctx *context.Context) {
	file := ctx.FormString("file")
	attach, err := repo_model.GetAttachmentByUUID(ctx, file)
	if err != nil {
		ctx.HTTPError(http.StatusBadRequest, err.Error())
		return
	}
	if !ctx.IsSigned || (ctx.Doer.ID != attach.UploaderID) {
		ctx.HTTPError(http.StatusForbidden)
		return
	}
	err = repo_model.DeleteAttachment(ctx, attach, true)
	if err != nil {
		ctx.HTTPError(http.StatusInternalServerError, fmt.Sprintf("DeleteAttachment: %v", err))
		return
	}
	ctx.JSON(http.StatusOK, map[string]string{
		"uuid": attach.UUID,
	})
}

// unlinkedAttachmentRepoReadable reports whether an attachment that is not linked to an issue
// or release (carrying only a RepoID) may be served to the current user based on their read
// permission for the owning repository. This covers file/article editor uploads (the intended
// case), but also any other repo-scoped unlinked attachment such as a pending issue/release
// draft. Because the originating unit can no longer be recovered without the link, it gates on
// unit.TypeCode as a deliberately conservative default; the uploader-only fallback in
// ServeAttachment still applies when this returns false.
func unlinkedAttachmentRepoReadable(ctx *context.Context, attach *repo_model.Attachment) bool {
	if attach.RepoID == 0 {
		return false
	}
	repo, err := repo_model.GetRepositoryByID(ctx, attach.RepoID)
	if err != nil || repo == nil {
		return false
	}
	perm, err := access_model.GetUserRepoPermission(ctx, repo, ctx.Doer)
	if err != nil {
		return false
	}
	return perm.CanRead(unit.TypeCode)
}

// GetAttachment serve attachments with the given UUID
func ServeAttachment(ctx *context.Context, uuid string) {
	attach, err := repo_model.GetAttachmentByUUID(ctx, uuid)
	if err != nil {
		if repo_model.IsErrAttachmentNotExist(err) {
			ctx.HTTPError(http.StatusNotFound)
		} else {
			ctx.ServerError("GetAttachmentByUUID", err)
		}
		return
	}

	repository, unitType, err := repo_service.LinkedRepository(ctx, attach)
	if err != nil {
		ctx.ServerError("LinkedRepository", err)
		return
	}

	if repository == nil { // If not linked to an issue or release
		// Editor/article attachments carry only a RepoID (no issue/release). Authorize them by
		// repository read permission so article readers can view embedded images; otherwise fall
		// back to uploader-only for genuinely context-less uploads (e.g. pending comment drafts).
		if !unlinkedAttachmentRepoReadable(ctx, attach) && !(ctx.IsSigned && attach.UploaderID == ctx.Doer.ID) {
			ctx.HTTPError(http.StatusNotFound)
			return
		}
	} else { // If we have the repository we check access
		perm, err := access_model.GetUserRepoPermission(ctx, repository, ctx.Doer)
		if err != nil {
			ctx.HTTPError(http.StatusInternalServerError, "GetUserRepoPermission", err.Error())
			return
		}
		if !perm.CanRead(unitType) {
			ctx.HTTPError(http.StatusNotFound)
			return
		}
	}

	if err := attach.IncreaseDownloadCount(ctx); err != nil {
		ctx.ServerError("IncreaseDownloadCount", err)
		return
	}

	if setting.Attachment.Storage.ServeDirect() {
		// If we have a signed url (S3, object storage), redirect to this directly.
		u, err := storage.Attachments.URL(attach.RelativePath(), attach.Name, ctx.Req.Method, nil)

		if u != nil && err == nil {
			ctx.Redirect(u.String())
			return
		}
	}

	if httpcache.HandleGenericETagCache(ctx.Req, ctx.Resp, `"`+attach.UUID+`"`) {
		return
	}

	// If we have matched and access to release or issue
	fr, err := storage.Attachments.Open(attach.RelativePath())
	if err != nil {
		ctx.ServerError("Open", err)
		return
	}
	defer fr.Close()

	common.ServeContentByReadSeeker(ctx.Base, attach.Name, util.ToPointer(attach.CreatedUnix.AsTime()), fr)
}

// GetAttachment serve attachments
func GetAttachment(ctx *context.Context) {
	ServeAttachment(ctx, ctx.PathParam("uuid"))
}
