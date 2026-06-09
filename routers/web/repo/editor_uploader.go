// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/context/upload"
	files_service "code.gitea.io/gitea/services/repository/files"
)

// isUploadSizeError reports whether err is caused by an upload that exceeded the
// configured request size limit or was truncated mid-stream. It relies on
// structured errors (http.MaxBytesError, io.EOF) rather than fragile string
// matching, with a narrow fallback for multipart's premature-end error.
func isUploadSizeError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	// mime/multipart reports a body that ends prematurely as a plain (unwrappable) error.
	msg := err.Error()
	return strings.Contains(msg, "multipart: NextPart: EOF") || strings.Contains(msg, "unexpected EOF")
}

// uploadTooLargeMessage is a user-facing message that avoids leaking internal error details.
func uploadTooLargeMessage() string {
	return fmt.Sprintf("The uploaded file is too large (limit %d MB).", setting.Attachment.MaxSize)
}

// UploadFileToServer upload file to server file dir not git
func UploadFileToServer(ctx *context.Context) {
	file, header, err := ctx.Req.FormFile("file")
	if err != nil {
		if isUploadSizeError(err) {
			ctx.HTTPError(http.StatusRequestEntityTooLarge, uploadTooLargeMessage())
			return
		}
		ctx.ServerError("FormFile", err)
		return
	}
	defer file.Close()

	if header.Size > setting.Attachment.MaxSize<<20 {
		ctx.HTTPError(http.StatusRequestEntityTooLarge, uploadTooLargeMessage())
		return
	}

	buf := make([]byte, 1024)
	n, _ := util.ReadAtMost(file, buf)
	if n > 0 {
		buf = buf[:n]
	}

	err = upload.Verify(buf, header.Filename, setting.Repository.Upload.AllowedTypes)
	if err != nil {
		ctx.HTTPError(http.StatusBadRequest, err.Error())
		return
	}

	name := files_service.CleanGitTreePath(header.Filename)
	if len(name) == 0 {
		ctx.HTTPError(http.StatusBadRequest, "Upload file name is invalid")
		return
	}

	uploaded, err := repo_model.NewUpload(ctx, name, buf, file)
	if err != nil {
		ctx.ServerError("NewUpload", err)
		return
	}

	ctx.JSON(http.StatusOK, map[string]string{"uuid": uploaded.UUID})
}

// RemoveUploadFileFromServer remove file from server file dir
func RemoveUploadFileFromServer(ctx *context.Context) {
	fileUUID := ctx.FormString("file")
	if err := repo_model.DeleteUploadByUUID(ctx, fileUUID); err != nil {
		ctx.ServerError("DeleteUploadByUUID", err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
