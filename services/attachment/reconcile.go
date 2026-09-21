// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"
	"fmt"

	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	"code.gitea.io/gitea/modules/log"
)

// ReconcileOptions configures a reconciliation run.
type ReconcileOptions struct {
	// DryRun reports what is missing without writing any association.
	DryRun bool
	// BatchSize is how many repositories are loaded per page.
	BatchSize int
	// StartRepoID resumes an interrupted run at that repository ID.
	StartRepoID int64
}

// ReconcileResult reports what a run found.
type ReconcileResult struct {
	BackfillResult
	// DanglingAssociations counts associations whose attachment row is gone.
	// They are reported, never removed.
	DanglingAssociations int64
}

// ReconcileArticleAttachments repairs associations that should exist and do
// not: a push whose association write failed after the commit succeeded, an
// interrupted backfill, a fork whose copy did not complete.
//
// It is strictly add-only. An association is never removed because the current
// tip no longer shows the URL — an article that dropped an image keeps the
// attachment alive, since an older revision of the article still shows it and
// the repository is still entitled to serve it. What it cannot repair it
// counts, so the discrepancy is visible instead of silently corrected.
func ReconcileArticleAttachments(ctx context.Context, opts ReconcileOptions) (*ReconcileResult, error) {
	result := &ReconcileResult{}

	backfilled, err := BackfillArticleAttachments(ctx, BackfillOptions{
		DryRun:      opts.DryRun,
		BatchSize:   opts.BatchSize,
		StartRepoID: opts.StartRepoID,
		// The aggregate notice belongs to the reconciliation run, which has
		// the dangling count to add to it.
		suppressReport: true,
	})
	if backfilled != nil {
		result.BackfillResult = *backfilled
	}
	if err != nil {
		return result, err
	}

	dangling, err := repo_model.CountDanglingArticleAttachments(ctx)
	if err != nil {
		return result, fmt.Errorf("count dangling article attachments: %w", err)
	}
	result.DanglingAssociations = dangling

	reportReconcile(result, opts.DryRun)
	return result, nil
}

// reportReconcile logs the counters of the whole run and raises at most one
// system notice for it. Per-reference detail is already in the warning log:
// one notice per reference would let a single bad paste fill the
// administrator's notice list.
func reportReconcile(result *ReconcileResult, dryRun bool) {
	log.Info("article attachment reconciliation: %d repositories scanned (%d unreadable), %d references found, %d associations inserted, %d legacy rows inferred, %d suspicious skipped, %d missing attachments, %d missing files, %d dangling associations, %d unassociated legacy attachments (dry run: %t)",
		result.ReposScanned, result.RepositoriesFailed, result.ReferencesFound, result.AssociationsInserted,
		result.LegacyInferred, result.SuspiciousSkipped, result.MissingAttachments, result.MissingFiles,
		result.DanglingAssociations, result.UnassociatedLegacy, dryRun)

	missing := result.MissingAttachments + result.MissingFiles
	if result.SuspiciousSkipped+missing+result.RepositoriesFailed == 0 && result.DanglingAssociations == 0 {
		return
	}
	// A dry run reports what a real run would repair, so it is worth a notice
	// too: it is how an operator checks the instance before enabling the
	// collector.
	if err := system_model.CreateRepositoryNotice("Article attachment reconciliation found %d suspicious references, %d missing attachments, %d missing stored objects, %d dangling associations and %d unreadable repositories, and inserted %d missing associations (dry run: %t), see the log for details",
		result.SuspiciousSkipped, result.MissingAttachments, result.MissingFiles,
		result.DanglingAssociations, result.RepositoriesFailed, result.AssociationsInserted, dryRun); err != nil {
		log.Error("CreateRepositoryNotice: %v", err)
	}
}
