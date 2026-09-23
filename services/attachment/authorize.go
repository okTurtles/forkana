// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"

	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
)

// DenyReason is the audit reason code reported when CanAssociate refuses a
// reference. It is meant for the per-reference warning log, which is the only
// channel refusals use: one system notice per refused UUID would let a single
// bad paste fill the administrator's notice list.
type DenyReason string

const (
	// DenyNone is reported together with an allowed decision, or with an error.
	DenyNone DenyReason = ""
	// DenyInvalidArgument reports a missing or unsaved repository/attachment.
	DenyInvalidArgument DenyReason = "invalid_argument"
	// DenyNotArticlePurpose reports an attachment that was not uploaded as
	// article content, typically a pending issue or release draft.
	DenyNotArticlePurpose DenyReason = "not_article_purpose"
	// DenyUnrelatedRepository reports an article attachment the target has no
	// trusted relationship with.
	DenyUnrelatedRepository DenyReason = "unrelated_repository"
)

// CanAssociate reports whether target may keep attach alive, i.e. whether an
// observed reference to attach found in target's article content may become an
// article association.
//
// Knowledge of a UUID is never authorization, and write permission on target is
// never sufficient on its own: a reference placed in an unrelated repository
// must not turn into ownership of somebody else's attachment. Only these
// relationships are trusted:
//
//  1. target, or an ancestor of target in the recorded fork chain, already holds
//     the association — an existing association is a recorded fact, so it is
//     trusted whatever the attachment was uploaded for;
//  2. the attachment was uploaded as article content and target is the origin
//     repository, or descends from it through the recorded fork chain;
//  3. the attachment was uploaded as article content by doer and is still
//     pending, i.e. not linked to an issue, comment or release and not
//     associated with any repository yet.
//
// An attachment of any other purpose is never inferred to be article content
// here: capturing a pending issue or release draft would expose it to everyone
// who can read the article. Legacy rows predating the purpose column are
// inferred by the backfill, which has the history and lineage context this
// predicate lacks.
func CanAssociate(ctx context.Context, doer *user_model.User, target *repo_model.Repository, attach *repo_model.Attachment) (bool, DenyReason, error) {
	if target == nil || attach == nil || target.ID == 0 || attach.ID == 0 {
		return false, DenyInvalidArgument, nil
	}

	associated, err := repo_model.HasArticleAttachment(ctx, target.ID, attach.ID)
	if err != nil {
		return false, DenyNone, err
	}
	if associated {
		return true, DenyNone, nil
	}

	isArticle := attach.Purpose == repo_model.AttachmentPurposeArticle
	if isArticle {
		if target.ID == attach.RepoID {
			return true, DenyNone, nil
		}

		if pending, err := isPendingUploadOf(ctx, doer, attach); err != nil {
			return false, DenyNone, err
		} else if pending {
			return true, DenyNone, nil
		}
	}

	inherits, err := inheritsThroughForkChain(ctx, target, attach, isArticle)
	if err != nil {
		return false, DenyNone, err
	}
	if inherits {
		return true, DenyNone, nil
	}
	if !isArticle {
		return false, DenyNotArticlePurpose, nil
	}
	return false, DenyUnrelatedRepository, nil
}

// isPendingUploadOf reports whether doer uploaded attach and no article commit
// or linked unit has claimed it yet.
func isPendingUploadOf(ctx context.Context, doer *user_model.User, attach *repo_model.Attachment) (bool, error) {
	if doer == nil || attach.UploaderID == 0 || doer.ID != attach.UploaderID {
		return false, nil
	}
	if attach.IssueID != 0 || attach.CommentID != 0 || attach.ReleaseID != 0 {
		return false, nil
	}
	count, err := repo_model.CountArticleAttachmentRepos(ctx, attach.ID)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// inheritsThroughForkChain walks target's fork ancestry looking for an already
// associated repository, or, when the attachment is article content, the origin
// repository, which is what makes a fork's inherited content trustworthy. The
// walk is bounded by the same setting that bounds fork trees, and stops at the
// first repository it cannot load.
func inheritsThroughForkChain(ctx context.Context, target *repo_model.Repository, attach *repo_model.Attachment, isArticle bool) (bool, error) {
	limit := setting.Repository.MaxForkTreeNodes
	if limit <= 0 {
		limit = 1
	}

	visited := make(map[int64]bool, 4)
	current := target
	for range limit {
		if !current.IsFork || current.ForkID == 0 || visited[current.ForkID] {
			return false, nil
		}
		visited[current.ForkID] = true

		if isArticle && current.ForkID == attach.RepoID {
			return true, nil
		}
		associated, err := repo_model.HasArticleAttachment(ctx, current.ForkID, attach.ID)
		if err != nil {
			return false, err
		}
		if associated {
			return true, nil
		}

		parent, err := repo_model.GetRepositoryByID(ctx, current.ForkID)
		if err != nil {
			if repo_model.IsErrRepoNotExist(err) {
				return false, nil
			}
			return false, err
		}
		current = parent
	}
	return false, nil
}
