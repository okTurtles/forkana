// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
	"code.gitea.io/gitea/modules/timeutil"
)

// GarbageCollectArticleAttachmentsOptions provides options for GarbageCollectArticleAttachments.
type GarbageCollectArticleAttachmentsOptions struct {
	// OlderThan is the grace period cutoff: attachments created later are left alone.
	OlderThan time.Time
	// Limit caps how many attachments a single run may reclaim. Zero means no cap.
	Limit int
	// DryRun reports what would be reclaimed without touching anything.
	DryRun bool
	// Force collects even while the legacy read fallback is still on, i.e. before the
	// association backfill has been finalized.
	Force bool
}

// DefaultArticleAttachmentGCGracePeriod is how old an unreferenced article upload must be
// before it is collected. An upload precedes the commit that references it, so the grace
// period must outlast that window; a week is ample, and short enough to keep abandoned
// uploads from accumulating.
const DefaultArticleAttachmentGCGracePeriod = 7 * 24 * time.Hour

// legacyArticleFallbackEnabled reports whether attachments are still authorized by the
// legacy read fallback. It is a variable so tests can switch it without waiting for the
// dynamic setting's cache to expire.
var legacyArticleFallbackEnabled = func(ctx context.Context) bool {
	return setting.Config().Attachment.LegacyArticleFallback.Value(ctx)
}

// ErrArticleAttachmentGCGated is returned when the collector refuses to run because the
// association backfill has not been finalized yet.
var ErrArticleAttachmentGCGated = errors.New("article attachment collection is gated until the association backfill is finalized")

// GarbageCollectArticleAttachments reclaims article attachments that no repository
// references any more. It returns how many attachments were collected.
//
// An article upload is created before the commit that associates it, and the
// association is written after the push succeeds, so a freshly uploaded attachment
// is legitimately unreferenced for a while. The grace period covers that window,
// and the zero-reference condition is re-checked by the database as part of the
// delete, so an association committed in between keeps the attachment alive.
//
// Until the association backfill is finalized, a live article attachment may still
// lack its association, so the collector does nothing and returns
// ErrArticleAttachmentGCGated unless opts.Force is set. Upgraded instances therefore
// start collecting on their own once `gitea admin backfill-article-attachments
// --finalize` succeeds, and fresh installations, which start finalized, right away.
func GarbageCollectArticleAttachments(ctx context.Context, opts GarbageCollectArticleAttachmentsOptions) (int, error) {
	log.Trace("Doing: GarbageCollectArticleAttachments")
	defer log.Trace("Finished: GarbageCollectArticleAttachments")

	if !opts.Force && legacyArticleFallbackEnabled(ctx) {
		return 0, ErrArticleAttachmentGCGated
	}

	cutoff := timeutil.TimeStamp(opts.OlderThan.Unix())
	candidates, err := repo_model.FindUnreferencedArticleAttachments(ctx, cutoff, opts.Limit)
	if err != nil {
		return 0, fmt.Errorf("find unreferenced article attachments: %w", err)
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	if opts.DryRun {
		log.Info("GarbageCollectArticleAttachments: %d unreferenced article attachments older than %s (dry run)", len(candidates), opts.OlderThan.Format(time.RFC3339))
		return 0, nil
	}

	collected := 0
	for _, attach := range candidates {
		deleted, err := repo_model.DeleteUnreferencedArticleAttachment(ctx, attach.ID)
		if err != nil {
			return collected, fmt.Errorf("delete unreferenced article attachment [%d]: %w", attach.ID, err)
		}
		if !deleted {
			// A repository claimed it while this run was in flight.
			log.Debug("GarbageCollectArticleAttachments: attachment %d was associated meanwhile, keeping it", attach.ID)
			continue
		}
		// The row is gone, so nothing can hand the object out any more: removing it now
		// cannot strand a live attachment, while the reverse order could.
		system_model.RemoveStorageWithNotice(ctx, storage.Attachments, "Delete unreferenced article attachment", attach.RelativePath())
		collected++
	}

	log.Info("GarbageCollectArticleAttachments: collected %d/%d unreferenced article attachments", collected, len(candidates))
	return collected, nil
}
