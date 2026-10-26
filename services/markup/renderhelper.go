// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"context"
	"fmt"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/cache"
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

// subjectIDForLink returns the ID of the subject a Forkana internal link names, 0 when there
// is none. The name is looked up as written, then normalized the way the subject routes
// redirect it (RedirectToCanonicalSubject), so "solar system" finds "Solar system".
func subjectIDForLink(ctx context.Context, subject string) (int64, error) {
	found, err := repo_model.GetSubjectByName(ctx, subject)
	if repo_model.IsErrSubjectNotExist(err) {
		if canonical := subjecttitle.Normalize(subject); canonical != "" && canonical != subject {
			found, err = repo_model.GetSubjectByName(ctx, canonical)
		}
	}
	if repo_model.IsErrSubjectNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("subject %q: %w", subject, err)
	}
	return found.ID, nil
}

// isSubjectLinkExisting reports whether the subject a Forkana internal link points to has an
// article the viewer can open (public, live, with content, by a visible owner), and when
// owner is set, whether that user has one (issue #399): ownerExists is subjectExists when
// owner is empty. It makes one subject lookup (shared by the request's links to the subject,
// through the request's context cache) and one or two article checks. A lookup error is not
// a missing subject: the link stays a normal link.
func isSubjectLinkExisting(ctx context.Context, subject, owner string) (subjectExists, ownerExists bool) {
	subjectID, err := cache.GetWithContextCache(ctx, "forkana-subject-link", subject, subjectIDForLink)
	if err != nil {
		log.Error("isSubjectLinkExisting: %v", err)
		return true, true
	}
	if subjectID == 0 {
		return false, false
	}
	var doer *user.User
	if giteaCtx := gitea_context.GetWebContext(ctx); giteaCtx != nil {
		doer = giteaCtx.Doer
	}
	if owner != "" {
		has, err := repo_model.HasReadableArticle(ctx, subjectID, owner, doer)
		if err != nil {
			log.Error("isSubjectLinkExisting(%q, %q): %v", subject, owner, err)
			return true, true
		}
		if has {
			return true, true // the owner's article is one of the subject's
		}
	}
	has, err := repo_model.HasReadableArticle(ctx, subjectID, "", doer)
	if err != nil {
		log.Error("isSubjectLinkExisting(%q): %v", subject, err)
		return true, true
	}
	return has, has && owner == ""
}
