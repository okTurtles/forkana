// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/globallock"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
)

// ErrPendingQuotaExceeded means the uploader already holds as many pending article
// uploads, or as many pending bytes, as setting.Attachment allows.
type ErrPendingQuotaExceeded struct {
	MaxFiles int64
	MaxMB    int64
}

// IsErrPendingQuotaExceeded checks if an error is ErrPendingQuotaExceeded.
func IsErrPendingQuotaExceeded(err error) bool {
	return errors.As(err, new(ErrPendingQuotaExceeded))
}

func (err ErrPendingQuotaExceeded) Error() string {
	if err.MaxFiles > 0 {
		return fmt.Sprintf("too many pending article uploads: at most %d may await a commit", err.MaxFiles)
	}
	return fmt.Sprintf("pending article uploads exceed %d MB: commit or remove some first", err.MaxMB)
}

// ErrUploadRateLimited means the uploader made too many article uploads recently.
type ErrUploadRateLimited struct {
	Limit  int64
	Window time.Duration
}

// IsErrUploadRateLimited checks if an error is ErrUploadRateLimited.
func IsErrUploadRateLimited(err error) bool {
	return errors.As(err, new(ErrUploadRateLimited))
}

func (err ErrUploadRateLimited) Error() string {
	return fmt.Sprintf("too many article uploads: at most %d per %s", err.Limit, err.Window)
}

// quotaLimitedReader fails the upload as soon as it would push the uploader past the
// pending byte quota, so the excess is never written to storage.
type quotaLimitedReader struct {
	r         io.Reader
	remaining int64
	maxMB     int64
}

func (q *quotaLimitedReader) Read(p []byte) (int, error) {
	// Ask for one byte more than allowed: getting it back means the quota is exceeded.
	if int64(len(p)) > q.remaining+1 {
		p = p[:q.remaining+1]
	}
	n, err := q.r.Read(p)
	if int64(n) > q.remaining {
		return int(q.remaining), ErrPendingQuotaExceeded{MaxMB: q.maxMB}
	}
	q.remaining -= int64(n)
	return n, err
}

// UploadArticleAttachment stores an article editor upload after charging it against the
// uploader's rate limit and pending quota (setting.Attachment.Article*).
//
// The article editor uploads before the edit is submitted, and any signed-in reader may
// use it so that "fork and edit" works, so an upload that no commit ever claims costs its
// uploader nothing else. These limits bound what one account can hold until the
// gc_article_attachments task reclaims abandoned uploads.
//
// The checks and the insert run under a per-uploader lock: without it, concurrent uploads
// would all pass the check before any of them is counted. Only the same uploader's
// requests are serialized. The lock is shared across processes only when the instance
// configures a Redis global lock; the default memory lock covers a single process.
func UploadArticleAttachment(ctx context.Context, doer *user_model.User, file io.Reader, allowedTypes string, fileSize int64, attach *repo_model.Attachment) (*repo_model.Attachment, error) {
	attach.Purpose = repo_model.AttachmentPurposeArticle
	if doer != nil && doer.IsAdmin {
		return UploadAttachment(ctx, file, allowedTypes, fileSize, attach)
	}

	release, err := globallock.Lock(ctx, fmt.Sprintf("article_attachment_upload_%d", attach.UploaderID))
	if err != nil {
		return nil, fmt.Errorf("lock article uploads of user %d: %w", attach.UploaderID, err)
	}
	defer release()

	file, err = chargeArticleUpload(ctx, attach.UploaderID, file, fileSize)
	if err != nil {
		return nil, err
	}
	return UploadAttachment(ctx, file, allowedTypes, fileSize, attach)
}

// chargeArticleUpload rejects the upload when the uploader is over the rate limit or the
// pending quota, and otherwise returns file wrapped so that its bytes cannot outgrow
// the remaining pending byte quota.
func chargeArticleUpload(ctx context.Context, uploaderID int64, file io.Reader, fileSize int64) (io.Reader, error) {
	cfg := setting.Attachment

	if cfg.ArticleUploadRateLimit > 0 && cfg.ArticleUploadRateWindow > 0 {
		since := timeutil.TimeStamp(time.Now().Add(-cfg.ArticleUploadRateWindow).Unix())
		recent, err := repo_model.CountArticleUploadsSince(ctx, uploaderID, since)
		if err != nil {
			return nil, fmt.Errorf("count recent article uploads: %w", err)
		}
		if recent >= cfg.ArticleUploadRateLimit {
			return nil, ErrUploadRateLimited{Limit: cfg.ArticleUploadRateLimit, Window: cfg.ArticleUploadRateWindow}
		}
	}

	if cfg.ArticleMaxPendingFiles <= 0 && cfg.ArticleMaxPendingSize <= 0 {
		return file, nil
	}
	count, size, err := repo_model.GetPendingArticleAttachmentStats(ctx, uploaderID)
	if err != nil {
		return nil, fmt.Errorf("get pending article uploads: %w", err)
	}
	if cfg.ArticleMaxPendingFiles > 0 && count >= cfg.ArticleMaxPendingFiles {
		return nil, ErrPendingQuotaExceeded{MaxFiles: cfg.ArticleMaxPendingFiles}
	}
	if cfg.ArticleMaxPendingSize > 0 {
		remaining := cfg.ArticleMaxPendingSize<<20 - size
		// A declared size is only a hint, so the reader enforces the quota on the real bytes.
		if remaining <= 0 || fileSize > remaining {
			return nil, ErrPendingQuotaExceeded{MaxMB: cfg.ArticleMaxPendingSize}
		}
		file = &quotaLimitedReader{r: file, remaining: remaining, maxMB: cfg.ArticleMaxPendingSize}
	}
	return file, nil
}
