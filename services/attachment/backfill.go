// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"
	"fmt"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
)

// DefaultBackfillBatchSize is how many repositories a backfill loads at a time.
const DefaultBackfillBatchSize = 50

// BackfillOptions configures a backfill run.
type BackfillOptions struct {
	// DryRun scans and reports without writing any association.
	DryRun bool
	// Verify is a dry run whose purpose is to answer whether the legacy read
	// fallback may be switched off. It counts what is still outstanding.
	Verify bool
	// BatchSize is how many repositories are loaded per page.
	BatchSize int
	// StartRepoID resumes an interrupted run at that repository ID.
	StartRepoID int64
}

// BackfillResult reports what a run found. Per-reference detail goes to the
// logs; these counters are what an operator acts on.
type BackfillResult struct {
	ReposScanned         int
	ReferencesFound      int
	AssociationsInserted int
	LegacyInferred       int
	SuspiciousSkipped    int
	MissingAttachments   int
	MissingFiles         int
	// RepositoriesFailed counts the repositories whose article history could
	// not be read at all, typically a broken or unreadable git directory.
	RepositoriesFailed int
	// Outstanding counts the references a read-only run would have
	// associated. Finalizing the transition requires it to be zero.
	Outstanding int
	// UnassociatedLegacy counts the unspecified-purpose attachments no
	// repository keeps alive. They are advisory: the scan cannot prove whether
	// something still reaches them through the legacy fallback.
	UnassociatedLegacy int64
	// LastRepoID is the last repository processed, to resume from after a failure.
	LastRepoID int64
}

// BackfillArticleAttachments walks every repository, reads the attachment
// references out of its retained article history and records the associations
// that predate the association table.
//
// The run is idempotent and restartable: repositories are processed in ID
// order, insertion is conflict-safe, and a failure reports the last repository
// it got through so the next run can resume there.
func BackfillArticleAttachments(ctx context.Context, opts BackfillOptions) (*BackfillResult, error) {
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBackfillBatchSize
	}
	readOnly := opts.DryRun || opts.Verify

	result := &BackfillResult{}
	nextID := opts.StartRepoID
	for {
		repos, err := repositoriesFrom(ctx, nextID, batchSize)
		if err != nil {
			return result, fmt.Errorf("list repositories from %d: %w", nextID, err)
		}
		if len(repos) == 0 {
			break
		}
		for _, repo := range repos {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if err := backfillRepository(ctx, repo, readOnly, result); err != nil {
				return result, fmt.Errorf("backfill [repo: %s, id: %d]: %w", repo.FullName(), repo.ID, err)
			}
			result.ReposScanned++
			result.LastRepoID = repo.ID
		}
		nextID = repos[len(repos)-1].ID + 1
	}

	unassociated, err := repo_model.CountUnassociatedLegacyAttachments(ctx)
	if err != nil {
		return result, fmt.Errorf("count unassociated legacy attachments: %w", err)
	}
	result.UnassociatedLegacy = unassociated

	reportBackfill(result, readOnly)
	return result, nil
}

// FinalizeLegacyFallback switches attachment authorization to associations
// only, but refuses while a verification still finds references that would
// lose their access.
func FinalizeLegacyFallback(ctx context.Context, opts BackfillOptions) (*BackfillResult, error) {
	opts.Verify, opts.DryRun, opts.StartRepoID = true, true, 0
	result, err := BackfillArticleAttachments(ctx, opts)
	if err != nil {
		return result, err
	}
	if result.Outstanding > 0 {
		return result, fmt.Errorf("%d article attachment references are not associated yet, run the backfill before finalizing", result.Outstanding)
	}
	if result.MissingAttachments > 0 {
		log.Warn("finalizing with %d references to attachments that no longer exist; they were already broken", result.MissingAttachments)
	}
	if err := system_model.SetSettings(ctx, map[string]string{
		setting.Config().Attachment.LegacyArticleFallback.DynKey(): "false",
	}); err != nil {
		return result, fmt.Errorf("disable the legacy article attachment fallback: %w", err)
	}
	log.Info("legacy article attachment fallback disabled; %d unassociated legacy attachments are now unreachable", result.UnassociatedLegacy)
	return result, nil
}

// backfillRepository records the associations one repository's article history
// justifies. References it may not claim are counted and logged, never
// associated: a reference is an observation, and only a trusted relationship
// turns it into access.
func backfillRepository(ctx context.Context, repo *repo_model.Repository, readOnly bool, result *BackfillResult) error {
	if repo.IsEmpty {
		return nil
	}
	if exists, err := gitrepo.IsRepositoryExist(ctx, repo); err != nil {
		return fmt.Errorf("IsRepositoryExist: %w", err)
	} else if !exists {
		log.Warn("article attachment backfill: repository %s has no git directory, skipped", repo.FullName())
		return nil
	}

	uuids, err := scanArticleHistoryUUIDs(ctx, repo.RepoPath())
	if err != nil {
		// A repository whose git data cannot be read must not stop the run:
		// the walk is add-only, so the remaining repositories still gain their
		// associations, and this one is reported for a later attempt.
		result.RepositoriesFailed++
		log.Error("article attachment backfill: cannot scan the article history of %s: %v", repo.FullName(), err)
		return nil
	}
	if len(uuids) == 0 {
		return nil
	}
	result.ReferencesFound += len(uuids)

	attachments, err := repo_model.GetAttachmentsByUUIDs(ctx, uuids)
	if err != nil {
		return fmt.Errorf("GetAttachmentsByUUIDs: %w", err)
	}
	found := make(container.Set[string], len(attachments))
	for _, attach := range attachments {
		found.Add(attach.UUID)
	}
	for _, attachmentUUID := range uuids {
		if !found.Contains(attachmentUUID) {
			result.MissingAttachments++
			log.Warn("article attachment backfill: %s references attachment %s, which does not exist", repo.FullName(), attachmentUUID)
		}
	}

	associated, err := repo_model.GetRepoArticleAttachmentIDs(ctx, repo.ID)
	if err != nil {
		return fmt.Errorf("GetRepoArticleAttachmentIDs: %w", err)
	}
	known := container.SetOf(associated...)

	toAssociate := make([]int64, 0, len(attachments))
	toInfer := make([]int64, 0, len(attachments))
	for _, attach := range attachments {
		if known.Contains(attach.ID) {
			continue
		}
		decision, err := classifyBackfillReference(ctx, repo, attach)
		if err != nil {
			return err
		}
		if decision == backfillSkip {
			result.SuspiciousSkipped++
			continue
		}
		if _, err := storage.Attachments.Stat(attach.RelativePath()); err != nil {
			result.MissingFiles++
			log.Warn("article attachment backfill: %s references attachment %s, whose stored object is missing: %v", repo.FullName(), attach.UUID, err)
		}
		toAssociate = append(toAssociate, attach.ID)
		if decision == backfillInferLegacy {
			toInfer = append(toInfer, attach.ID)
		}
	}
	if len(toAssociate) == 0 {
		return nil
	}
	if readOnly {
		result.Outstanding += len(toAssociate)
		return nil
	}

	if err := db.WithTx(ctx, func(ctx context.Context) error {
		if err := repo_model.AddArticleAttachments(ctx, repo.ID, toAssociate); err != nil {
			return fmt.Errorf("AddArticleAttachments: %w", err)
		}
		// An inferred row becomes a first-class article attachment, so its
		// lifetime follows the associations from now on instead of staying
		// outside the collector's reach forever.
		if _, err := repo_model.MarkAttachmentsArticlePurpose(ctx, toInfer); err != nil {
			return fmt.Errorf("MarkAttachmentsArticlePurpose: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	result.AssociationsInserted += len(toAssociate)
	result.LegacyInferred += len(toInfer)
	return nil
}

// backfillDecision is what a run concluded about one observed reference.
type backfillDecision int

const (
	// backfillSkip marks a suspicious reference: observed, never associated.
	backfillSkip backfillDecision = iota
	// backfillAssociate marks a reference the live predicate already trusts.
	backfillAssociate
	// backfillInferLegacy marks a row predating the purpose column that this
	// repository's own history identifies as article content.
	backfillInferLegacy
)

// classifyBackfillReference decides whether a reference may become an
// association, and whether that decision rests on legacy inference.
//
// The live predicate answers first. Only when it refuses does the backfill add
// what it alone has: the knowledge that this repository's own article history
// carries the reference. That justifies treating an unspecified-purpose row as
// article content, but only for the repository it was uploaded to or a
// descendant of it — never for an unrelated repository that merely pasted the
// URL.
func classifyBackfillReference(ctx context.Context, repo *repo_model.Repository, attach *repo_model.Attachment) (backfillDecision, error) {
	allowed, reason, err := CanAssociate(ctx, nil, repo, attach)
	if err != nil {
		return backfillSkip, fmt.Errorf("CanAssociate [attachment: %d]: %w", attach.ID, err)
	}
	if allowed {
		return backfillAssociate, nil
	}

	inferred, err := inferredLegacyArticleAttachment(ctx, repo, attach)
	if err != nil {
		return backfillSkip, err
	}
	if inferred {
		return backfillInferLegacy, nil
	}
	log.Warn("article attachment backfill: %s references attachment %s (%d), which it may not claim (%s), skipped",
		repo.FullName(), attach.UUID, attach.ID, reason)
	return backfillSkip, nil
}

// inferredLegacyArticleAttachment reports whether an attachment predating the
// purpose column may be treated as this repository's article content. A row an
// issue, comment or release claims is never inferred: capturing it would expose
// it to every reader of the article.
func inferredLegacyArticleAttachment(ctx context.Context, repo *repo_model.Repository, attach *repo_model.Attachment) (bool, error) {
	if attach.Purpose != repo_model.AttachmentPurposeUnspecified || attach.RepoID == 0 {
		return false, nil
	}
	if attach.IssueID != 0 || attach.CommentID != 0 || attach.ReleaseID != 0 {
		return false, nil
	}
	if repo.ID == attach.RepoID {
		return true, nil
	}
	return inheritsThroughForkChain(ctx, repo, attach, true)
}

// repositoriesFrom pages repositories by ID, which is what makes a run
// resumable: an ID never changes and never reappears behind the cursor.
func repositoriesFrom(ctx context.Context, startID int64, limit int) ([]*repo_model.Repository, error) {
	repos := make([]*repo_model.Repository, 0, limit)
	return repos, db.GetEngine(ctx).Where("id >= ?", startID).OrderBy("id ASC").Limit(limit).Find(&repos)
}

func reportBackfill(result *BackfillResult, readOnly bool) {
	log.Info("article attachment backfill: %d repositories scanned (%d unreadable), %d references found, %d associations inserted, %d legacy rows inferred, %d suspicious skipped, %d missing attachments, %d missing files, %d outstanding, %d unassociated legacy attachments (read-only: %t)",
		result.ReposScanned, result.RepositoriesFailed, result.ReferencesFound, result.AssociationsInserted,
		result.LegacyInferred, result.SuspiciousSkipped, result.MissingAttachments, result.MissingFiles,
		result.Outstanding, result.UnassociatedLegacy, readOnly)

	// One notice for the whole run: a notice per rejected reference would let a
	// single bad paste fill the administrator's notice list.
	reportable := result.SuspiciousSkipped + result.MissingAttachments + result.MissingFiles + result.RepositoriesFailed
	if readOnly || reportable == 0 {
		return
	}
	if err := system_model.CreateRepositoryNotice("Article attachment backfill finished with %d suspicious references skipped, %d missing attachments, %d missing files and %d unreadable repositories, see the log for details",
		result.SuspiciousSkipped, result.MissingAttachments, result.MissingFiles, result.RepositoriesFailed); err != nil {
		log.Error("CreateRepositoryNotice: %v", err)
	}
}
