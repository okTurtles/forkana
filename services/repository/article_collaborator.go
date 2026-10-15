// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/perm"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/util"
	notify_service "code.gitea.io/gitea/services/notify"
)

var (
	// ErrArticleCollaboratorExists is returned when inviting a user already listed on the article.
	ErrArticleCollaboratorExists = util.NewAlreadyExistErrorf("user is already a collaborator of the article")
	// ErrArticleCollaboratorOwnsSubject is returned when the invitee owns another active article
	// for the same subject: the one-article-per-subject rule also applies to maintainers.
	ErrArticleCollaboratorOwnsSubject = util.NewInvalidArgumentErrorf("user already owns an article for this subject")
	// ErrArticleCollaboratorNotEligible is returned when the invitee cannot collaborate at all
	// (the owner, an organization, an inactive or a blocked user).
	ErrArticleCollaboratorNotEligible = util.NewInvalidArgumentErrorf("user cannot collaborate on the article")
	// ErrArticleCollaboratorNoInvite is returned when the doer has no pending invitation.
	ErrArticleCollaboratorNoInvite = util.NewNotExistErrorf("no pending article collaborator invitation")
	// ErrArticleArchived is returned when inviting or accepting an invitation to an archived
	// article: it takes no new collaborators, while the existing ones can still have their
	// role changed or be removed.
	ErrArticleArchived = util.NewPermissionDeniedErrorf("article is archived")
)

// ArticleCollaboratorPermission is what a user may do with the collaborators of an article.
type ArticleCollaboratorPermission struct {
	// IsOwner is true for the article owner, who has full control over its collaborators.
	IsOwner bool
	// CanManage is true when the user may invite and remove collaborators, the owner and
	// accepted delegates.
	CanManage bool
}

// CanGrantRole reports whether the user may invite a collaborator with the given role:
// a delegate may only grant the plain maintainer role, never delegation itself.
func (p ArticleCollaboratorPermission) CanGrantRole(role repo_model.ArticleCollaboratorRole) bool {
	if !role.IsValid() || !p.CanManage {
		return false
	}
	return p.IsOwner || !role.IsDelegate()
}

// CanChangeRoles reports whether the user may change the role of existing collaborators.
func (p ArticleCollaboratorPermission) CanChangeRoles() bool {
	return p.IsOwner
}

// CanRemove reports whether the user may remove the collaborator (or withdraw their
// invitation): the owner may remove anyone, a delegate only plain maintainers.
func (p ArticleCollaboratorPermission) CanRemove(c *repo_model.ArticleCollaborator) bool {
	if !p.CanManage {
		return false
	}
	return p.IsOwner || !c.Role.IsDelegate()
}

// GetArticleCollaboratorPermission returns what the doer may do with the collaborators of
// the article. A signed-out user, or one who is neither the owner nor an accepted delegate,
// gets no permission.
func GetArticleCollaboratorPermission(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) (ArticleCollaboratorPermission, error) {
	if doer == nil {
		return ArticleCollaboratorPermission{}, nil
	}
	if repo.OwnerID == doer.ID {
		return NewArticleCollaboratorPermission(true, nil), nil
	}
	c, err := repo_model.GetArticleCollaborator(ctx, repo.ID, doer.ID)
	if err != nil {
		return ArticleCollaboratorPermission{}, err
	}
	return NewArticleCollaboratorPermission(false, c), nil
}

// NewArticleCollaboratorPermission builds the permission of a user from whether they own the
// article and their collaborator row on it (nil when they have none), for callers that
// already loaded that row.
func NewArticleCollaboratorPermission(isOwner bool, c *repo_model.ArticleCollaborator) ArticleCollaboratorPermission {
	if isOwner {
		return ArticleCollaboratorPermission{IsOwner: true, CanManage: true}
	}
	return ArticleCollaboratorPermission{CanManage: c != nil && c.IsAccepted() && c.Role.IsDelegate()}
}

// CanUserMergeArticleChangeRequest reports whether the doer may merge change requests of
// the article: its owner and its accepted maintainers.
func CanUserMergeArticleChangeRequest(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) (bool, error) {
	if doer == nil {
		return false, nil
	}
	if repo.OwnerID == doer.ID {
		return true, nil
	}
	return repo_model.IsAcceptedArticleMaintainer(ctx, repo.ID, doer.ID)
}

// InviteArticleCollaborator invites a user to maintain the article with the given role.
// The invitation grants nothing until the invitee accepts it.
func InviteArticleCollaborator(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, invitee *user_model.User, role repo_model.ArticleCollaboratorRole) error {
	if repo.IsArchived {
		return ErrArticleArchived
	}

	p, err := GetArticleCollaboratorPermission(ctx, doer, repo)
	if err != nil {
		return err
	}
	if !p.CanGrantRole(role) {
		return util.NewPermissionDeniedErrorf("cannot grant article collaborator role %d", role)
	}

	if invitee.ID == repo.OwnerID || invitee.IsOrganization() || !invitee.IsActive || invitee.ProhibitLogin {
		return ErrArticleCollaboratorNotEligible
	}
	if err := repo.LoadOwner(ctx); err != nil {
		return err
	}
	if user_model.IsUserBlockedBy(ctx, invitee, repo.OwnerID) || user_model.IsUserBlockedBy(ctx, repo.Owner, invitee.ID) {
		return ErrArticleCollaboratorNotEligible
	}
	// a delegate inviting is checked too: the invitee is notified by them
	if doer.ID != repo.OwnerID && (user_model.IsUserBlockedBy(ctx, invitee, doer.ID) || user_model.IsUserBlockedBy(ctx, doer, invitee.ID)) {
		return ErrArticleCollaboratorNotEligible
	}
	if err := checkArticleCollaboratorSubject(ctx, repo, invitee); err != nil {
		return err
	}

	existing, err := repo_model.GetArticleCollaborator(ctx, repo.ID, invitee.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		return ErrArticleCollaboratorExists
	}

	if err := repo_model.CreateArticleCollaborator(ctx, &repo_model.ArticleCollaborator{
		RepoID:    repo.ID,
		UserID:    invitee.ID,
		InviterID: doer.ID,
		Role:      role,
		Status:    repo_model.ArticleCollaboratorStatusPending,
	}); err != nil {
		if db.IsErrDuplicateKey(err) {
			return ErrArticleCollaboratorExists
		}
		return err
	}

	notify_service.ArticleCollaboratorInvited(ctx, doer, invitee, repo, role)
	return nil
}

// checkArticleCollaboratorSubject refuses a user who already owns an active article for the
// subject, whatever its fork relation to this one: maintaining is limited by the same
// one-article-per-subject rule as owning.
func checkArticleCollaboratorSubject(ctx context.Context, repo *repo_model.Repository, u *user_model.User) error {
	if repo.SubjectID == 0 {
		return nil
	}
	ownRepo, err := repo_model.GetActiveRepositoryByOwnerIDAndSubjectID(ctx, u.ID, repo.SubjectID)
	if err != nil {
		return err
	}
	if ownRepo != nil {
		return ErrArticleCollaboratorOwnsSubject
	}
	return nil
}

// AcceptArticleCollaboratorInvite accepts the doer's pending invitation and grants them
// write access to the article.
func AcceptArticleCollaboratorInvite(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) error {
	if repo.IsArchived {
		return ErrArticleArchived
	}

	return db.WithTx(ctx, func(ctx context.Context) error {
		c, err := getPendingArticleCollaboratorInvite(ctx, repo.ID, doer.ID)
		if err != nil {
			return err
		}
		// the invitee may have created their own article for the subject since the invitation
		if err := checkArticleCollaboratorSubject(ctx, repo, doer); err != nil {
			return err
		}

		c.Status = repo_model.ArticleCollaboratorStatusAccepted
		if err := repo_model.UpdateArticleCollaboratorCols(ctx, c, "status"); err != nil {
			if errors.Is(err, util.ErrNotExist) {
				// withdrawn concurrently
				return ErrArticleCollaboratorNoInvite
			}
			return fmt.Errorf("accept article collaborator invitation: %w", err)
		}
		if err := AddOrUpdateCollaborator(ctx, repo, doer, perm.AccessModeWrite); err != nil {
			return fmt.Errorf("AddOrUpdateCollaborator: %w", err)
		}
		return activities_model.MarkArticleCollaboratorInviteNotificationsRead(ctx, repo.ID, doer.ID)
	})
}

// DeclineArticleCollaboratorInvite declines the doer's pending invitation.
func DeclineArticleCollaboratorInvite(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		c, err := getPendingArticleCollaboratorInvite(ctx, repo.ID, doer.ID)
		if err != nil {
			return err
		}
		if err := repo_model.DeleteArticleCollaboratorRow(ctx, c); err != nil {
			if errors.Is(err, util.ErrNotExist) {
				// accepted or withdrawn concurrently
				return ErrArticleCollaboratorNoInvite
			}
			return err
		}
		return activities_model.MarkArticleCollaboratorInviteNotificationsRead(ctx, repo.ID, doer.ID)
	})
}

// getPendingArticleCollaboratorInvite returns the user's pending invitation to the article,
// or ErrArticleCollaboratorNoInvite when there is none.
func getPendingArticleCollaboratorInvite(ctx context.Context, repoID, userID int64) (*repo_model.ArticleCollaborator, error) {
	c, err := repo_model.GetArticleCollaborator(ctx, repoID, userID)
	if err != nil {
		return nil, err
	}
	if c == nil || !c.IsPending() {
		return nil, ErrArticleCollaboratorNoInvite
	}
	return c, nil
}

// ChangeArticleCollaboratorRole changes the role of a collaborator, pending or accepted.
// Both roles map to the same write access, so only the article record changes.
func ChangeArticleCollaboratorRole(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, userID int64, role repo_model.ArticleCollaboratorRole) error {
	if !role.IsValid() {
		return util.NewInvalidArgumentErrorf("invalid article collaborator role %d", role)
	}

	p, err := GetArticleCollaboratorPermission(ctx, doer, repo)
	if err != nil {
		return err
	}
	if !p.CanChangeRoles() {
		return util.NewPermissionDeniedErrorf("cannot change article collaborator roles")
	}

	c, err := repo_model.GetArticleCollaborator(ctx, repo.ID, userID)
	if err != nil {
		return err
	}
	if c == nil {
		return util.NewNotExistErrorf("article collaborator %d does not exist", userID)
	}
	if c.Role == role {
		return nil
	}
	c.Role = role
	return repo_model.UpdateArticleCollaboratorCols(ctx, c, "role")
}

// RemoveArticleCollaborators removes the given users from the article collaborators,
// withdrawing pending invitations and revoking the write access of accepted ones. Either
// every user is removed or none: a single user the doer may not remove fails the call.
func RemoveArticleCollaborators(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}

	p, err := GetArticleCollaboratorPermission(ctx, doer, repo)
	if err != nil {
		return err
	}
	if !p.CanManage {
		return util.NewPermissionDeniedErrorf("cannot manage article collaborators")
	}

	return db.WithTx(ctx, func(ctx context.Context) error {
		for _, userID := range userIDs {
			c, err := repo_model.GetArticleCollaborator(ctx, repo.ID, userID)
			if err != nil {
				return err
			}
			if c == nil {
				continue
			}
			if !p.CanRemove(c) {
				return util.NewPermissionDeniedErrorf("cannot remove article collaborator %d", userID)
			}
			if err := removeArticleCollaborator(ctx, repo, c); err != nil {
				return err
			}
		}
		return nil
	})
}

// WithdrawPendingArticleCollaboratorInvites withdraws every pending invitation of the
// article, called when it is archived: an archived article takes no new collaborators, so
// the invitations could no longer be accepted.
func WithdrawPendingArticleCollaboratorInvites(ctx context.Context, repo *repo_model.Repository) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		if err := repo_model.DeletePendingArticleCollaborators(ctx, repo.ID); err != nil {
			return err
		}
		// accepting marks the invitee's notifications read, so the unread ones left are
		// exactly those of the withdrawn invitations
		return activities_model.MarkRepoArticleCollaboratorInviteNotificationsRead(ctx, repo.ID)
	})
}

// RemoveArticleCollaboratorsOfOwner removes the user from the collaborators of every article
// owned by ownerID, withdrawing pending invitations and revoking accepted ones, called when
// either of them blocks the other.
func RemoveArticleCollaboratorsOfOwner(ctx context.Context, ownerID, userID int64) error {
	collaborators, err := repo_model.GetArticleCollaboratorsOfOwner(ctx, ownerID, userID)
	if err != nil {
		return err
	}
	for _, c := range collaborators {
		// the Gitea collaboration is removed by the block itself
		if err := repo_model.DeleteArticleCollaboratorRow(ctx, c); err != nil {
			return err
		}
		if err := activities_model.MarkArticleCollaboratorInviteNotificationsRead(ctx, c.RepoID, userID); err != nil {
			return err
		}
	}
	return nil
}

func removeArticleCollaborator(ctx context.Context, repo *repo_model.Repository, c *repo_model.ArticleCollaborator) error {
	// an invitation accepted concurrently fails the delete: removing it as pending would
	// leave the write access just granted in place
	if err := repo_model.DeleteArticleCollaboratorRow(ctx, c); err != nil {
		return err
	}
	if c.IsPending() {
		return activities_model.MarkArticleCollaboratorInviteNotificationsRead(ctx, repo.ID, c.UserID)
	}

	u, err := user_model.GetUserByID(ctx, c.UserID)
	if err != nil {
		if user_model.IsErrUserNotExist(err) {
			return nil
		}
		return err
	}
	return DeleteCollaboration(ctx, repo, u)
}
