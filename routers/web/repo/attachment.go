// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"fmt"
	"net/http"

	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/httpcache"
	"code.gitea.io/gitea/modules/httplib"
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
	uploadAttachment(ctx, ctx.Repo.Repository.ID, setting.Attachment.AllowedTypes, repo_model.AttachmentPurposeUnspecified)
}

// UploadReleaseAttachment response for uploading release attachments
func UploadReleaseAttachment(ctx *context.Context) {
	uploadAttachment(ctx, ctx.Repo.Repository.ID, setting.Repository.Release.AllowedTypes, repo_model.AttachmentPurposeUnspecified)
}

// UploadEditorAttachment response for images pasted or dropped in the article/file editor.
// It differs from UploadIssueAttachment only in the recorded purpose, which is what lets the
// article-attachment garbage collector tell an abandoned editor upload apart from an unfinished
// issue or release draft.
func UploadEditorAttachment(ctx *context.Context) {
	uploadAttachment(ctx, ctx.Repo.Repository.ID, setting.Attachment.AllowedTypes, repo_model.AttachmentPurposeArticle)
}

// UploadAttachment response for uploading attachments
func uploadAttachment(ctx *context.Context, repoID int64, allowedTypes string, purpose repo_model.AttachmentPurpose) {
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
		Purpose:    purpose,
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
		// Absolute (scheme+host), not app-relative: markdown's link resolver treats a
		// root-relative "/attachments/{uuid}" as relative to the current file's ref/path
		// base and mangles it into "/{owner}/{repo}/media/branch/{ref}/attachments/{uuid}"
		// (a 404) once the file is committed and rendered. A full URL is recognized as
		// already-absolute and passed through untouched.
		"url": httplib.MakeAbsoluteURL(ctx, "/attachments/"+attach.UUID),
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
	// An attachment referenced by committed article content is shared: the repositories that
	// reference it — including forks the uploader has no say over — would lose the blob. Only
	// dropping the last association may delete it, which is the garbage collector's job.
	associations, err := repo_model.CountArticleAttachmentRepos(ctx, attach.ID)
	if err != nil {
		ctx.HTTPError(http.StatusInternalServerError, fmt.Sprintf("CountArticleAttachmentRepos: %v", err))
		return
	}
	if associations > 0 {
		ctx.HTTPError(http.StatusConflict, "attachment is referenced by article content")
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

// attachmentServingScope returns the repository named by the request URL, when the route has
// one. "/{username}/{reponame}/attachments/{uuid}" already carries the assigned repository,
// while "/article/{username}/{subjectname}/attachments/{uuid}" is served without
// RepoAssignment — opening the git repository and counting branches, tags and releases for
// every embedded image is not worth it — so the owner/subject pair is resolved directly here.
//
// A scope that cannot be resolved yields nil, which degrades the request to the unscoped rule
// rather than to a 404: the same caller reaches the very same attachment through the global
// "/attachments/{uuid}" route.
func attachmentServingScope(ctx *context.Context) *repo_model.Repository {
	if ctx.Repo != nil && ctx.Repo.Repository != nil {
		return ctx.Repo.Repository
	}
	ownerName, subjectName := ctx.PathParam("username"), ctx.PathParam("subjectname")
	if ownerName == "" || subjectName == "" {
		return nil
	}
	repo, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, ownerName, subjectName)
	if err != nil {
		if !repo_model.IsErrRepoNotExist(err) && !repo_model.IsErrSubjectNotExist(err) {
			log.Error("GetRepositoryByOwnerAndSubject [owner: %s, subject: %s]: %v", ownerName, subjectName, err)
		}
		return nil
	}
	return repo
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
		// Article attachments are shared across the repositories associated with them, so
		// access follows those associations rather than the repository they were uploaded to.
		canServe, err := attachment.CanServe(ctx, ctx.Doer, attachmentServingScope(ctx), attach)
		if err != nil {
			ctx.ServerError("CanServe", err)
			return
		}
		if !canServe {
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
