// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"context"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/subjecttitle"
	gitea_context "code.gitea.io/gitea/services/context"
)

func FormalRenderHelperFuncs() *markup.RenderHelperFuncs {
	return &markup.RenderHelperFuncs{
		RenderRepoFileCodePreview: renderRepoFileCodePreview,
		RenderRepoIssueIconTitle:  renderRepoIssueIconTitle,
		IsSubjectLinkExisting:     isSubjectLinkExisting,
		IsUsernameMentionable: func(ctx context.Context, username string) bool {
			mentionedUser, err := user.GetUserByName(ctx, username)
			if err != nil {
				return false
			}

			giteaCtx := gitea_context.GetWebContext(ctx)
			if giteaCtx == nil {
				// when using general context, use user's visibility to check
				return mentionedUser.Visibility.IsPublic()
			}

			// when using gitea context (web context), use user's visibility and user's permission to check
			return user.IsUserVisibleToViewer(giteaCtx, mentionedUser, giteaCtx.Doer)
		},
	}
}

// isSubjectLinkExisting reports whether the subject a Forkana internal link points to has a
// live (not deleted) public article, and when owner is set, whether that user has an
// article on it (issue #399). Names are looked up as written, then normalized the way the
// subject routes redirect them (RedirectToCanonicalSubject), so "solar system" finds
// "Solar system". A lookup error is not a missing subject: the link stays a normal link.
func isSubjectLinkExisting(ctx context.Context, subject, owner string) bool {
	repo, err := repo_model.GetPublicRepositoryBySubject(ctx, subject)
	if err != nil && (repo_model.IsErrSubjectNotExist(err) || repo_model.IsErrRepoWithSubjectNotExist(err)) {
		if canonical := subjecttitle.Normalize(subject); canonical != "" && canonical != subject {
			subject = canonical
			repo, err = repo_model.GetPublicRepositoryBySubject(ctx, subject)
		}
	}
	if err != nil {
		if repo_model.IsErrSubjectNotExist(err) || repo_model.IsErrRepoWithSubjectNotExist(err) {
			return false
		}
		log.Error("isSubjectLinkExisting(%q): %v", subject, err)
		return true
	}
	if repo.IsTombstone() {
		return false
	}
	if owner == "" {
		return true
	}
	article, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, owner, subject)
	if err != nil {
		if repo_model.IsErrRepoNotExist(err) || repo_model.IsErrSubjectNotExist(err) {
			return false
		}
		log.Error("isSubjectLinkExisting(%q, %q): %v", subject, owner, err)
		return true
	}
	return !article.IsPrivate && !article.IsTombstone()
}
