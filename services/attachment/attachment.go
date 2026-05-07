// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context/upload"

	"github.com/google/uuid"
)

// limitedErrorReader wraps a reader and returns ErrFileTooLarge if more than remaining bytes are read.
// Unlike io.LimitReader, it errors instead of silently stopping, preventing silent file truncation
// when fileSize is unknown (-1).
type limitedErrorReader struct {
	r         io.Reader
	remaining int64
	name      string
	maxMB     int64
}

func (l *limitedErrorReader) Read(p []byte) (int, error) {
	// Request one extra byte beyond the limit. If we get it back, the file exceeds the limit.
	allowRead := l.remaining + 1
	if int64(len(p)) > allowRead {
		p = p[:allowRead]
	}
	n, err := l.r.Read(p)
	if int64(n) > l.remaining {
		return int(l.remaining), upload.ErrFileTooLarge{Name: l.name, MaxMB: l.maxMB}
	}
	l.remaining -= int64(n)
	return n, err
}

// NewAttachment creates a new attachment object, but do not verify.
func NewAttachment(ctx context.Context, attach *repo_model.Attachment, file io.Reader, size int64) (*repo_model.Attachment, error) {
	if attach.RepoID == 0 {
		return nil, fmt.Errorf("attachment %s should belong to a repository", attach.Name)
	}

	err := db.WithTx(ctx, func(ctx context.Context) error {
		attach.UUID = uuid.New().String()
		size, err := storage.Attachments.Save(attach.RelativePath(), file, size)
		if err != nil {
			return fmt.Errorf("Create: %w", err)
		}
		attach.Size = size

		return db.Insert(ctx, attach)
	})

	return attach, err
}

// UploadAttachment upload new attachment into storage and update database
func UploadAttachment(ctx context.Context, file io.Reader, allowedTypes string, fileSize int64, attach *repo_model.Attachment) (*repo_model.Attachment, error) {
	if setting.Attachment.MaxSize > 0 {
		maxBytes := setting.Attachment.MaxSize << 20
		if fileSize > maxBytes {
			return nil, upload.ErrFileTooLarge{Name: attach.Name, MaxMB: setting.Attachment.MaxSize}
		}
		// Wrap with a reader that errors (rather than silently truncates) when the limit is exceeded.
		// This handles both spoofed Content-Length and unknown size (fileSize == -1).
		file = &limitedErrorReader{r: file, remaining: maxBytes, name: attach.Name, maxMB: setting.Attachment.MaxSize}
	}

	buf := make([]byte, 1024)
	n, _ := util.ReadAtMost(file, buf)
	buf = buf[:n]

	if err := upload.Verify(buf, attach.Name, allowedTypes); err != nil {
		return nil, err
	}

	return NewAttachment(ctx, attach, io.MultiReader(bytes.NewReader(buf), file), fileSize)
}

// UpdateAttachment updates an attachment, verifying that its name is among the allowed types.
func UpdateAttachment(ctx context.Context, allowedTypes string, attach *repo_model.Attachment) error {
	if err := upload.Verify(nil, attach.Name, allowedTypes); err != nil {
		return err
	}

	return repo_model.UpdateAttachment(ctx, attach)
}
