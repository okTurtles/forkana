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

// isSubjectLinkExisting reports whether the subject a Forkana internal link points to has an
// article the viewer can open (public, live, with content, by a visible owner), and when
// owner is set, whether that user has one (issue #399). Names are looked up as written, then
// normalized the way the subject routes redirect them (RedirectToCanonicalSubject), so
// "solar system" finds "Solar system". A lookup error is not a missing subject: the link
// stays a normal link.
func isSubjectLinkExisting(ctx context.Context, subject, owner string) bool {
	found, err := repo_model.GetSubjectByName(ctx, subject)
	if repo_model.IsErrSubjectNotExist(err) {
		if canonical := subjecttitle.Normalize(subject); canonical != "" && canonical != subject {
			found, err = repo_model.GetSubjectByName(ctx, canonical)
		}
	}
	if err != nil {
		if repo_model.IsErrSubjectNotExist(err) {
			return false
		}
		log.Error("isSubjectLinkExisting(%q): %v", subject, err)
		return true
	}
	var doer *user.User
	if giteaCtx := gitea_context.GetWebContext(ctx); giteaCtx != nil {
		doer = giteaCtx.Doer
	}
	exists, err := repo_model.HasReadableArticle(ctx, found.ID, owner, doer)
	if err != nil {
		log.Error("isSubjectLinkExisting(%q, %q): %v", subject, owner, err)
		return true
	}
	return exists
}
