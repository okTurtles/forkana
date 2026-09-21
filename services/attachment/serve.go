// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"context"

	access_model "code.gitea.io/gitea/models/perm/access"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
)

// CanServe reports whether doer may download an attachment that is not linked
// to an issue, comment or release. Linked attachments keep being authorized by
// their linked repository and unit, which this predicate never touches.
//
// An article attachment is shared by every repository associated with it, so
// access follows the associations rather than the repository it happened to be
// uploaded to: a fork's reader must be able to read the inherited image even
// though the origin repository may be unreadable to them, or gone.
//
// scope is the repository named by the request URL, when there is one. It only
// narrows: the named repository must itself hold the association, so a
// repository cannot be used as a cover to serve an attachment it does not keep
// alive. A request with no resolvable scope falls back to the global rule,
// which the same caller can reach through "/attachments/{uuid}" anyway.
func CanServe(ctx context.Context, doer *user_model.User, scope *repo_model.Repository, attach *repo_model.Attachment) (bool, error) {
	if attach == nil || attach.ID == 0 {
		return false, nil
	}

	// The uploader always reaches their own upload, including the pending one
	// the editor is still previewing, which no repository references yet.
	if doer != nil && doer.ID != 0 && doer.ID == attach.UploaderID {
		return true, nil
	}

	if scope != nil && scope.ID != 0 {
		associated, err := repo_model.HasArticleAttachment(ctx, scope.ID, attach.ID)
		if err != nil {
			return false, err
		}
		if associated {
			return repoReadable(ctx, doer, scope)
		}
	} else {
		accessible, err := repo_model.ArticleAttachmentAccessible(ctx, doer, attach.ID)
		if err != nil {
			return false, err
		}
		if accessible {
			return true, nil
		}
	}

	return legacyFallbackServable(ctx, doer, attach)
}

// legacyFallbackServable authorizes an attachment that has no association at
// all by the read permission of the repository it was uploaded to. It is the
// transitional path for rows predating the association table: enabled on
// upgraded instances until the backfill is finalized, disabled on fresh ones.
//
// It is confined to attachments of unspecified purpose. An article upload
// records its purpose, so a purposeful article attachment without associations
// is a pending upload, which only its uploader may see.
func legacyFallbackServable(ctx context.Context, doer *user_model.User, attach *repo_model.Attachment) (bool, error) {
	if attach.Purpose != repo_model.AttachmentPurposeUnspecified || attach.RepoID == 0 {
		return false, nil
	}
	if !setting.Config().Attachment.LegacyArticleFallback.Value(ctx) {
		return false, nil
	}

	count, err := repo_model.CountArticleAttachmentRepos(ctx, attach.ID)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}

	repo, err := repo_model.GetRepositoryByID(ctx, attach.RepoID)
	if err != nil {
		if repo_model.IsErrRepoNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return repoReadable(ctx, doer, repo)
}

// repoReadable gates on unit.TypeCode: an unlinked attachment no longer records
// the unit it was uploaded for, and article content is served from the code
// unit, so that is the conservative unit to require.
func repoReadable(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) (bool, error) {
	perm, err := access_model.GetUserRepoPermission(ctx, repo, doer)
	if err != nil {
		return false, err
	}
	return perm.CanRead(unit.TypeCode), nil
}
