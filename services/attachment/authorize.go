// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"

	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
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
//  1. target already holds the association;
//  2. target is the origin repository the attachment was uploaded to;
//  3. doer uploaded the attachment and it is still pending, i.e. not linked to
//     an issue, comment or release and not associated with any repository yet;
//  4. target descends, through the recorded fork chain, from a repository that
//     is the origin or already holds the association.
func CanAssociate(ctx context.Context, doer *user_model.User, target *repo_model.Repository, attach *repo_model.Attachment) (bool, error) {
	if target == nil || attach == nil || target.ID == 0 || attach.ID == 0 {
		return false, nil
	}

	if target.ID == attach.RepoID {
		return true, nil
	}

	associated, err := repo_model.HasArticleAttachment(ctx, target.ID, attach.ID)
	if err != nil {
		return false, err
	}
	if associated {
		return true, nil
	}

	if pending, err := isPendingUploadOf(ctx, doer, attach); err != nil {
		return false, err
	} else if pending {
		return true, nil
	}

	return inheritsThroughForkChain(ctx, target, attach)
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

// inheritsThroughForkChain walks target's fork ancestry looking for the origin
// repository or an already associated one, which is what makes a fork's
// inherited content trustworthy. The walk is bounded by the same setting that
// bounds fork trees, and stops at the first repository it cannot load.
func inheritsThroughForkChain(ctx context.Context, target *repo_model.Repository, attach *repo_model.Attachment) (bool, error) {
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

		if current.ForkID == attach.RepoID {
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
