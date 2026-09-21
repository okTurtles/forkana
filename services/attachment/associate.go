// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"
	"fmt"

	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/markup/attachmentref"
)

// ArticleContentPaths are the tree paths scanned for attachment references when
// only the branch tip is known, as is the case for a direct Git push. It is the
// path the article editor writes; richer discovery belongs to reconciliation.
var ArticleContentPaths = []string{"README.md"}

// AssociateArticleAttachments records that repo keeps alive every attachment
// referenced by the given article contents and allowed by CanAssociate. It must
// only be called once the ref update it describes has succeeded, because an
// association outlives the content that introduced it.
//
// Insertion is idempotent, so the same content discovered twice — once by the
// web commit path and once by the push path — is harmless. References the doer
// may not associate are skipped, not rejected: the push already happened, and
// skipping merely withholds access.
func AssociateArticleAttachments(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, contents ...string) error {
	if repo == nil || repo.ID == 0 {
		return nil
	}

	uuids := make([]string, 0, 8)
	seen := make(container.Set[string], 8)
	for _, content := range contents {
		for _, uuid := range attachmentref.ExtractAttachmentUUIDs(content) {
			if seen.Add(uuid) {
				uuids = append(uuids, uuid)
			}
		}
	}
	if len(uuids) == 0 {
		return nil
	}

	attachments, err := repo_model.GetAttachmentsByUUIDs(ctx, uuids)
	if err != nil {
		return fmt.Errorf("GetAttachmentsByUUIDs: %w", err)
	}

	attachmentIDs := make([]int64, 0, len(attachments))
	for _, attach := range attachments {
		allowed, reason, err := CanAssociate(ctx, doer, repo, attach)
		if err != nil {
			return fmt.Errorf("CanAssociate [attachment: %d]: %w", attach.ID, err)
		}
		if !allowed {
			log.Warn("article attachment %s referenced by %s is not associable (%s), skipped", attach.UUID, repo.FullName(), reason)
			continue
		}
		attachmentIDs = append(attachmentIDs, attach.ID)
	}
	if len(attachmentIDs) == 0 {
		return nil
	}

	if err := repo_model.AddArticleAttachments(ctx, repo.ID, attachmentIDs); err != nil {
		return fmt.Errorf("AddArticleAttachments: %w", err)
	}
	return nil
}

// AssociateArticleAttachmentsFromCommit reads the given tree paths out of a
// committed tree and associates the attachments they reference.
//
// The content is read back from the commit rather than taken from the caller's
// buffers: it is the only form that is identical for a web commit and for a
// direct push, and it never mistakes an LFS pointer for article text. Paths
// that do not resolve — a deletion, or a repository that is not an article —
// are skipped.
func AssociateArticleAttachmentsFromCommit(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, commit *git.Commit, treePaths []string) error {
	if commit == nil || len(treePaths) == 0 {
		return nil
	}

	contents := make([]string, 0, len(treePaths))
	for _, treePath := range treePaths {
		entry, err := commit.GetTreeEntryByPath(treePath)
		if err != nil {
			if git.IsErrNotExist(err) {
				continue
			}
			return fmt.Errorf("GetTreeEntryByPath [path: %s]: %w", treePath, err)
		}
		if !entry.Mode().IsRegular() {
			continue
		}
		content, err := entry.Blob().GetBlobContent(attachmentref.MaxScanSize)
		if err != nil {
			return fmt.Errorf("GetBlobContent [path: %s]: %w", treePath, err)
		}
		contents = append(contents, content)
	}

	return AssociateArticleAttachments(ctx, doer, repo, contents...)
}

// ReportAssociationFailure records that a ref update succeeded while its
// attachment associations did not. The push is never rolled back: the bytes are
// protected by the garbage collector's grace period and the add-only
// reconciliation task repairs the missing rows, so this needs to be visible to
// an administrator rather than fatal to the request.
func ReportAssociationFailure(repo *repo_model.Repository, ref string, err error) {
	log.Error("article attachment association failed after a successful ref update [repo: %s, ref: %s]: %v", repo.FullName(), ref, err)
	if noticeErr := system_model.CreateRepositoryNotice("Failed to associate article attachments for %s at %s, run reconciliation to repair: %v", repo.FullName(), ref, err); noticeErr != nil {
		log.Error("CreateRepositoryNotice: %v", noticeErr)
	}
}
