// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	"code.gitea.io/gitea/modules/log"
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
}

// GarbageCollectArticleAttachments reclaims article attachments that no repository
// references any more. It returns how many attachments were collected.
//
// An article upload is created before the commit that associates it, and the
// association is written after the push succeeds, so a freshly uploaded attachment
// is legitimately unreferenced for a while. The grace period covers that window,
// and the zero-reference condition is re-checked by the database as part of the
// delete, so an association committed in between keeps the attachment alive.
func GarbageCollectArticleAttachments(ctx context.Context, opts GarbageCollectArticleAttachmentsOptions) (int, error) {
	log.Trace("Doing: GarbageCollectArticleAttachments")
	defer log.Trace("Finished: GarbageCollectArticleAttachments")

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
