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

// UploadFileToServer upload file to server file dir not git
func UploadFileToServer(ctx *context.Context) {
	file, header, err := ctx.Req.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "multipart: NextPart: EOF") ||
			strings.Contains(err.Error(), "request body too large") ||
			strings.Contains(err.Error(), "unexpected EOF") ||
			errors.Is(err, io.ErrUnexpectedEOF) ||
			errors.Is(err, io.EOF) {
			ctx.HTTPError(http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		ctx.ServerError("FormFile", err)
		return
	}
	defer file.Close()

	if header.Size > setting.UI.MaxDisplayFileSize {
		ctx.HTTPError(http.StatusRequestEntityTooLarge, fmt.Sprintf("File size exceeds the limit of %d MB", setting.UI.MaxDisplayFileSize/(1024*1024)))
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
