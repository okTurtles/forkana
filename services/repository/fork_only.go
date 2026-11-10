// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"

	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/util"
)

// ErrArticleForkOnly is returned when opening a change request against an article nobody
// can review the change requests of (see IsArticleForkOnly): the user must fork it instead.
var ErrArticleForkOnly = util.NewPermissionDeniedErrorf("the article does not accept change requests")

// UserAcceptsChangeRequests reports whether somebody reads the change requests sent to
// the user, see repo_model.UserAcceptsChangeRequests.
func UserAcceptsChangeRequests(u *user_model.User) bool {
	return repo_model.UserAcceptsChangeRequests(u)
}

// IsArticleForkOnly reports whether nobody can review the change requests of the
// article, so editing it must create the editor's own article (a fork) instead, see
// repo_model.IsArticleForkOnly. It returns the article's owner as well.
func IsArticleForkOnly(ctx context.Context, repo *repo_model.Repository) (bool, *user_model.User, error) {
	return repo_model.IsArticleForkOnly(ctx, repo)
}

// CheckArticleAcceptsChangeRequest returns ErrArticleForkOnly when the doer may not open a
// change request against the article because nobody can review it. Its owner and its
// maintainers, who can merge change requests, are never refused.
func CheckArticleAcceptsChangeRequest(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) error {
	refuses, err := repo_model.ArticleRefusesChangeRequestsFrom(ctx, repo, doer)
	if err != nil {
		return err
	}
	if refuses {
		return ErrArticleForkOnly
	}
	return nil
}
