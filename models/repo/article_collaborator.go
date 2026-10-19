// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"code.gitea.io/gitea/models/db"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"

	"xorm.io/builder"
)

// ArticleCollaboratorRole is the access level an article owner grants to a collaborator.
type ArticleCollaboratorRole int

const (
	// ArticleCollaboratorRoleMaintainer can edit the article directly and review and
	// merge its change requests.
	ArticleCollaboratorRoleMaintainer ArticleCollaboratorRole = iota + 1
	// ArticleCollaboratorRoleMaintainerDelegate has the maintainer rights and can also
	// invite and remove maintainers, but never other delegates.
	ArticleCollaboratorRoleMaintainerDelegate
)

// IsValid reports whether the role is one of the known roles.
func (r ArticleCollaboratorRole) IsValid() bool {
	return r == ArticleCollaboratorRoleMaintainer || r == ArticleCollaboratorRoleMaintainerDelegate
}

// IsDelegate reports whether the role can manage maintainers.
func (r ArticleCollaboratorRole) IsDelegate() bool {
	return r == ArticleCollaboratorRoleMaintainerDelegate
}

// LocaleKey returns the translation key of the role name.
func (r ArticleCollaboratorRole) LocaleKey() string {
	if r.IsDelegate() {
		return "repo.settings.article_collaborators_role_delegate"
	}
	return "repo.settings.article_collaborators_role_maintainer"
}

// ArticleCollaboratorStatus tells whether an invited collaborator has accepted yet.
type ArticleCollaboratorStatus int

const (
	// ArticleCollaboratorStatusPending is an invitation waiting for the invitee.
	ArticleCollaboratorStatusPending ArticleCollaboratorStatus = iota
	// ArticleCollaboratorStatusAccepted is an active collaborator.
	ArticleCollaboratorStatusAccepted
)

// ArticleCollaborator records a user invited to maintain an article, with their role
// and whether they accepted. It is the source of truth for the article collaborator
// UI: the Gitea "collaboration" row granting write access only exists once the
// invitation is accepted, so a pending invitee never gains any permission.
type ArticleCollaborator struct {
	ID          int64                     `xorm:"pk autoincr"`
	RepoID      int64                     `xorm:"UNIQUE(s) INDEX NOT NULL"`
	UserID      int64                     `xorm:"UNIQUE(s) INDEX NOT NULL"`
	InviterID   int64                     `xorm:"NOT NULL DEFAULT 0"`
	Role        ArticleCollaboratorRole   `xorm:"NOT NULL DEFAULT 1"`
	Status      ArticleCollaboratorStatus `xorm:"INDEX NOT NULL DEFAULT 0"`
	CreatedUnix timeutil.TimeStamp        `xorm:"created"`
	UpdatedUnix timeutil.TimeStamp        `xorm:"updated"`

	User    *user_model.User `xorm:"-"`
	Inviter *user_model.User `xorm:"-"`
}

func init() {
	db.RegisterModel(new(ArticleCollaborator))
}

// IsPending reports whether the invitation still waits for the invitee.
func (c *ArticleCollaborator) IsPending() bool {
	return c.Status == ArticleCollaboratorStatusPending
}

// IsAccepted reports whether the invitee accepted the invitation.
func (c *ArticleCollaborator) IsAccepted() bool {
	return c.Status == ArticleCollaboratorStatusAccepted
}

// LoadInviter loads the user who sent the invitation, a deleted inviter is shown as
// the ghost user.
func (c *ArticleCollaborator) LoadInviter(ctx context.Context) error {
	if c.Inviter != nil {
		return nil
	}
	inviter, err := user_model.GetPossibleUserByID(ctx, c.InviterID)
	if err != nil {
		if !user_model.IsErrUserNotExist(err) {
			return err
		}
		inviter = user_model.NewGhostUser()
	}
	c.Inviter = inviter
	return nil
}

// GetArticleCollaborator returns the collaborator row of a user on an article, or nil
// when the user was never invited.
func GetArticleCollaborator(ctx context.Context, repoID, userID int64) (*ArticleCollaborator, error) {
	c := &ArticleCollaborator{}
	has, err := db.GetEngine(ctx).Where(builder.Eq{"repo_id": repoID, "user_id": userID}).Get(c)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return c, nil
}

// IsAcceptedArticleMaintainer reports whether the user is an active collaborator of
// the article, in either role.
func IsAcceptedArticleMaintainer(ctx context.Context, repoID, userID int64) (bool, error) {
	if repoID == 0 || userID == 0 {
		return false, nil
	}
	return db.GetEngine(ctx).Where(builder.Eq{
		"repo_id": repoID,
		"user_id": userID,
		"status":  ArticleCollaboratorStatusAccepted,
	}).Exist(new(ArticleCollaborator))
}

// ListArticleCollaborators returns every collaborator of an article with their user
// loaded, pending invitations first and then by name. A collaborator whose account
// is gone is dropped, the user deletion removes their rows anyway.
func ListArticleCollaborators(ctx context.Context, repoID int64) ([]*ArticleCollaborator, error) {
	var collaborators []*ArticleCollaborator
	if err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Find(&collaborators); err != nil {
		return nil, fmt.Errorf("find article collaborators: %w", err)
	}
	if len(collaborators) == 0 {
		return collaborators, nil
	}

	userIDs := make([]int64, 0, len(collaborators))
	for _, c := range collaborators {
		userIDs = append(userIDs, c.UserID)
	}
	usersMap := make(map[int64]*user_model.User, len(userIDs))
	if err := db.GetEngine(ctx).In("id", userIDs).Find(&usersMap); err != nil {
		return nil, fmt.Errorf("find article collaborator users: %w", err)
	}

	result := make([]*ArticleCollaborator, 0, len(collaborators))
	for _, c := range collaborators {
		if c.User = usersMap[c.UserID]; c.User != nil {
			result = append(result, c)
		}
	}
	sortArticleCollaborators(result)
	return result, nil
}

func sortArticleCollaborators(collaborators []*ArticleCollaborator) {
	slices.SortFunc(collaborators, func(a, b *ArticleCollaborator) int {
		if a.Status != b.Status {
			return cmp.Compare(a.Status, b.Status)
		}
		return strings.Compare(a.User.LowerName, b.User.LowerName)
	})
}

// GetArticleCollaboratorsByUserIDs returns the collaborator rows of the given users on an
// article, users without one are left out.
func GetArticleCollaboratorsByUserIDs(ctx context.Context, repoID int64, userIDs []int64) ([]*ArticleCollaborator, error) {
	var collaborators []*ArticleCollaborator
	err := db.GetEngine(ctx).Where(builder.Eq{"repo_id": repoID}.And(builder.In("user_id", userIDs))).Find(&collaborators)
	return collaborators, err
}

// GetArticleCollaboratorsOfOwner returns the collaborator rows, pending or accepted, of a
// user on every article owned by ownerID.
func GetArticleCollaboratorsOfOwner(ctx context.Context, ownerID, userID int64) ([]*ArticleCollaborator, error) {
	var collaborators []*ArticleCollaborator
	err := db.GetEngine(ctx).
		Join("INNER", "repository", "repository.id = article_collaborator.repo_id").
		Where(builder.Eq{"repository.owner_id": ownerID, "article_collaborator.user_id": userID}).
		Find(&collaborators)
	return collaborators, err
}

// GetArticleCollaboratorUserIDs returns the IDs of every user listed on the article,
// pending or accepted.
func GetArticleCollaboratorUserIDs(ctx context.Context, repoID int64) ([]int64, error) {
	var ids []int64
	err := db.GetEngine(ctx).Table("article_collaborator").Where("repo_id = ?", repoID).Cols("user_id").Find(&ids)
	return ids, err
}

// CreateArticleCollaborator inserts a pending invitation.
func CreateArticleCollaborator(ctx context.Context, c *ArticleCollaborator) error {
	return db.Insert(ctx, c)
}

// UpdateArticleCollaboratorCols updates the given columns of a collaborator row. It fails
// with a not-exist error when the row is gone, so a transaction racing with a removal
// rolls back instead of acting on a deleted row.
func UpdateArticleCollaboratorCols(ctx context.Context, c *ArticleCollaborator, cols ...string) error {
	affected, err := db.GetEngine(ctx).ID(c.ID).Cols(cols...).Update(c)
	if err != nil {
		return err
	}
	if affected == 0 {
		return util.NewNotExistErrorf("article collaborator %d no longer exists", c.ID)
	}
	return nil
}

// DeleteArticleCollaboratorRow removes the row c was read from. It fails with a not-exist
// error when the row is gone or its status changed since, so a transaction racing with
// another answer to the invitation rolls back instead of leaving the write access granted
// alongside it out of step with the row.
func DeleteArticleCollaboratorRow(ctx context.Context, c *ArticleCollaborator) error {
	affected, err := db.GetEngine(ctx).Where(builder.Eq{"id": c.ID, "status": c.Status}).Delete(new(ArticleCollaborator))
	if err != nil {
		return err
	}
	if affected == 0 {
		return util.NewNotExistErrorf("article collaborator %d changed or no longer exists", c.ID)
	}
	return nil
}

// DeletePendingArticleCollaborators removes every pending invitation of an article.
func DeletePendingArticleCollaborators(ctx context.Context, repoID int64) error {
	_, err := db.GetEngine(ctx).Where(builder.Eq{
		"repo_id": repoID,
		"status":  ArticleCollaboratorStatusPending,
	}).Delete(new(ArticleCollaborator))
	return err
}

// DeleteArticleCollaborator removes a user's collaborator row from an article.
func DeleteArticleCollaborator(ctx context.Context, repoID, userID int64) error {
	_, err := db.GetEngine(ctx).Where(builder.Eq{"repo_id": repoID, "user_id": userID}).Delete(new(ArticleCollaborator))
	return err
}
