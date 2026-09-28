// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"
	"slices"

	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
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

// inheritsThroughForkChain reports whether target's fork ancestry holds the
// attachment: an already associated ancestor, or, when the attachment is
// article content, the origin repository, which is what makes a fork's
// inherited content trustworthy.
//
// The ancestry comes from repo_model.ForkAncestorIDs, so this walk and the fork
// tree walks share one depth bound, and both questions are then answered with a
// single query each rather than one per ancestor.
func inheritsThroughForkChain(ctx context.Context, target *repo_model.Repository, attach *repo_model.Attachment, isArticle bool) (bool, error) {
	if !target.IsFork || target.ForkID == 0 {
		return false, nil
	}

	ancestorIDs, err := repo_model.ForkAncestorIDs(ctx, target.ID)
	if err != nil {
		return false, err
	}
	if len(ancestorIDs) == 0 {
		return false, nil
	}
	if isArticle && slices.Contains(ancestorIDs, attach.RepoID) {
		return true, nil
	}
	return repo_model.AnyArticleAttachmentRepo(ctx, attach.ID, ancestorIDs)
}
