// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"errors"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/timeutil"
	repo_service "code.gitea.io/gitea/services/repository"
)

// pendingArticleAttachmentsTopUploaders is how many uploaders the report lists by size.
const pendingArticleAttachmentsTopUploaders = 10

// articleAttachmentGCGracePeriod returns the grace period gc_article_attachments uses, so
// the report counts exactly what the next collection would reclaim.
func articleAttachmentGCGracePeriod() time.Duration {
	config := &struct {
		OlderThan time.Duration
	}{OlderThan: repo_service.DefaultArticleAttachmentGCGracePeriod}
	if _, err := setting.GetCronSettings("gc_article_attachments", config); err != nil || config.OlderThan <= 0 {
		return repo_service.DefaultArticleAttachmentGCGracePeriod
	}
	return config.OlderThan
}

// checkPendingArticleAttachments reports article editor uploads that no commit ever
// claimed and that are old enough to be collected. The storage checks cannot see them:
// such an upload has both its database row and its stored object.
func checkPendingArticleAttachments(ctx context.Context, logger log.Logger, autofix bool) error {
	gracePeriod := articleAttachmentGCGracePeriod()
	olderThan := time.Now().Add(-gracePeriod)

	summary, err := repo_model.GetPendingArticleAttachmentSummary(ctx, timeutil.TimeStamp(olderThan.Unix()), pendingArticleAttachmentsTopUploaders)
	if err != nil {
		logger.Critical("Unable to summarize pending article attachments: %v", err)
		return err
	}
	if summary.Count == 0 {
		logger.Info("No pending article attachments older than %s", gracePeriod)
		return nil
	}

	logger.Warn("%d pending article attachments (%s) are older than %s and referenced by no repository", summary.Count, base.FileSize(summary.Size), gracePeriod)
	for _, uploader := range summary.TopUploaders {
		logger.Info("  uploader %d: %d attachments, %s", uploader.UploaderID, uploader.Count, base.FileSize(uploader.Size))
	}

	if !autofix {
		logger.Info("Run with --fix to collect them, or enable [cron.gc_article_attachments]")
		return nil
	}

	if err := storage.Init(); err != nil {
		logger.Error("storage.Init failed: %v", err)
		return err
	}
	collected, err := repo_service.GarbageCollectArticleAttachments(ctx, repo_service.GarbageCollectArticleAttachmentsOptions{
		OlderThan: olderThan,
	})
	if errors.Is(err, repo_service.ErrArticleAttachmentGCGated) {
		// Collecting before the backfill is finalized could delete a live attachment
		// whose association has not been recorded yet.
		logger.Warn("Not collecting: run `gitea admin backfill-article-attachments --finalize` first")
		return nil
	}
	if err != nil {
		logger.Critical("Unable to collect pending article attachments: %v", err)
		return err
	}
	logger.Info("Collected %d pending article attachments", collected)
	return nil
}

func init() {
	Register(&Check{
		Title:     "Check for abandoned article editor uploads",
		Name:      "article-attachments-pending",
		IsDefault: false,
		Run:       checkPendingArticleAttachments,
		Priority:  7,
	})
}
