// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	user_model "code.gitea.io/gitea/models/user"
)

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
func IsArticleForkOnly(ctx context.Context, repo *Repository) (bool, *user_model.User, error) {
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

	hasMaintainer, err := HasActiveArticleMaintainer(ctx, repo.ID)
	if err != nil {
		return false, nil, err
	}
	return !hasMaintainer, owner, nil
}

// ArticleRefusesChangeRequestsFrom reports whether the doer may not open a change request
// against the article because nobody can review it (see IsArticleForkOnly). Its owner and
// its accepted maintainers, who can merge change requests, are never refused.
func ArticleRefusesChangeRequestsFrom(ctx context.Context, repo *Repository, doer *user_model.User) (bool, error) {
	if doer != nil {
		if repo.OwnerID == doer.ID {
			return false, nil
		}
		isMaintainer, err := IsAcceptedArticleMaintainer(ctx, repo.ID, doer.ID)
		if err != nil || isMaintainer {
			return false, err
		}
	}
	forkOnly, _, err := IsArticleForkOnly(ctx, repo)
	return forkOnly, err
}
