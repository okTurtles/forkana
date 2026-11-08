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
// the user: not when the user is fork-only (like the accounts importing Wikipedia
// articles), deactivated or prohibited from signing in.
func UserAcceptsChangeRequests(u *user_model.User) bool {
	if u == nil {
		return false
	}
	if u.IsForkOnly {
		return false
	}
	// an organization is not a person who signs in, its members review for it
	if u.IsOrganization() {
		return true
	}
	return u.IsActive && !u.ProhibitLogin
}

// IsArticleForkOnly reports whether nobody can review the change requests of the
// article, so editing it must create the editor's own article (a fork) instead: its
// owner does not accept change requests (see UserAcceptsChangeRequests) and it has no
// active maintainer. It returns the article's owner as well, to name them to the editor.
// An owner whose account is gone is the ghost user, who accepts nothing. A repository
// without a subject is not an article, it is never fork-only.
func IsArticleForkOnly(ctx context.Context, repo *repo_model.Repository) (bool, *user_model.User, error) {
	if repo.SubjectID == 0 {
		return false, nil, nil
	}
	owner := user_model.NewGhostUser()
	if err := repo.LoadOwner(ctx); err == nil {
		owner = repo.Owner
	} else if !user_model.IsErrUserNotExist(err) {
		return false, nil, err
	}
	if UserAcceptsChangeRequests(owner) {
		return false, owner, nil
	}

	hasMaintainer, err := repo_model.HasActiveArticleMaintainer(ctx, repo.ID)
	if err != nil {
		return false, nil, err
	}
	return !hasMaintainer, owner, nil
}

// CheckArticleAcceptsChangeRequest returns ErrArticleForkOnly when the doer may not open a
// change request against the article because nobody can review it. Its owner and its
// maintainers, who can merge change requests, are never refused.
func CheckArticleAcceptsChangeRequest(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) error {
	canMerge, err := CanUserMergeArticleChangeRequest(ctx, doer, repo)
	if err != nil || canMerge {
		return err
	}
	forkOnly, _, err := IsArticleForkOnly(ctx, repo)
	if err != nil {
		return err
	}
	if forkOnly {
		return ErrArticleForkOnly
	}
	return nil
}
