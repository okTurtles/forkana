// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArticleCollaborators(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	// repo1 is an article (subject 1) owned by user2
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	delegate := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	maintainer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	other := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 8})

	t.Run("PendingInviteGrantsNothing", func(t *testing.T) {
		require.NoError(t, InviteArticleCollaborator(t.Context(), owner, repo, delegate, repo_model.ArticleCollaboratorRoleMaintainerDelegate))

		isCollaborator, err := repo_model.IsCollaborator(t.Context(), repo.ID, delegate.ID)
		require.NoError(t, err)
		assert.False(t, isCollaborator)

		perms, err := CheckForkOnEditPermissions(t.Context(), delegate, repo)
		require.NoError(t, err)
		assert.False(t, perms.CanEditDirectly)
		assert.True(t, perms.CanSubmitChangeRequest)

		p, err := GetArticleCollaboratorPermission(t.Context(), delegate, repo)
		require.NoError(t, err)
		assert.False(t, p.CanManage)

		canMerge, err := CanUserMergeArticleChangeRequest(t.Context(), repo, delegate)
		require.NoError(t, err)
		assert.False(t, canMerge)
	})

	t.Run("InviteTwiceRefused", func(t *testing.T) {
		err := InviteArticleCollaborator(t.Context(), owner, repo, delegate, repo_model.ArticleCollaboratorRoleMaintainer)
		assert.ErrorIs(t, err, ErrArticleCollaboratorExists)
	})

	t.Run("OwnerCannotBeInvited", func(t *testing.T) {
		err := InviteArticleCollaborator(t.Context(), owner, repo, owner, repo_model.ArticleCollaboratorRoleMaintainer)
		assert.ErrorIs(t, err, ErrArticleCollaboratorNotEligible)
	})

	t.Run("AcceptGrantsWriteAccess", func(t *testing.T) {
		require.NoError(t, AcceptArticleCollaboratorInvite(t.Context(), delegate, repo))

		isCollaborator, err := repo_model.IsCollaborator(t.Context(), repo.ID, delegate.ID)
		require.NoError(t, err)
		assert.True(t, isCollaborator)

		perms, err := CheckForkOnEditPermissions(t.Context(), delegate, repo)
		require.NoError(t, err)
		assert.True(t, perms.CanEditDirectly)
		assert.True(t, perms.IsArticleMaintainer)
		assert.False(t, perms.CanSubmitChangeRequest)

		canMerge, err := CanUserMergeArticleChangeRequest(t.Context(), repo, delegate)
		require.NoError(t, err)
		assert.True(t, canMerge)

		assert.ErrorIs(t, AcceptArticleCollaboratorInvite(t.Context(), delegate, repo), ErrArticleCollaboratorNoInvite)
	})

	t.Run("DelegateCannotGrantDelegation", func(t *testing.T) {
		err := InviteArticleCollaborator(t.Context(), delegate, repo, maintainer, repo_model.ArticleCollaboratorRoleMaintainerDelegate)
		assert.ErrorIs(t, err, util.ErrPermissionDenied)
	})

	t.Run("DelegateInvitesMaintainer", func(t *testing.T) {
		require.NoError(t, InviteArticleCollaborator(t.Context(), delegate, repo, maintainer, repo_model.ArticleCollaboratorRoleMaintainer))
		require.NoError(t, AcceptArticleCollaboratorInvite(t.Context(), maintainer, repo))

		c, err := repo_model.GetArticleCollaborator(t.Context(), repo.ID, maintainer.ID)
		require.NoError(t, err)
		assert.Equal(t, delegate.ID, c.InviterID)
		assert.True(t, c.IsAccepted())
	})

	t.Run("MaintainerCannotManage", func(t *testing.T) {
		err := InviteArticleCollaborator(t.Context(), maintainer, repo, other, repo_model.ArticleCollaboratorRoleMaintainer)
		assert.ErrorIs(t, err, util.ErrPermissionDenied)
		assert.ErrorIs(t, RemoveArticleCollaborators(t.Context(), maintainer, repo, []int64{delegate.ID}), util.ErrPermissionDenied)
	})

	t.Run("DelegateCannotChangeRolesOrRemoveDelegate", func(t *testing.T) {
		err := ChangeArticleCollaboratorRole(t.Context(), delegate, repo, maintainer.ID, repo_model.ArticleCollaboratorRoleMaintainerDelegate)
		assert.ErrorIs(t, err, util.ErrPermissionDenied)

		// a single forbidden user fails the whole removal
		err = RemoveArticleCollaborators(t.Context(), delegate, repo, []int64{maintainer.ID, delegate.ID})
		assert.ErrorIs(t, err, util.ErrPermissionDenied)
		unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
	})

	t.Run("OwnerChangesRole", func(t *testing.T) {
		require.NoError(t, ChangeArticleCollaboratorRole(t.Context(), owner, repo, maintainer.ID, repo_model.ArticleCollaboratorRoleMaintainerDelegate))
		c := unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
		assert.Equal(t, repo_model.ArticleCollaboratorRoleMaintainerDelegate, c.Role)
		require.NoError(t, ChangeArticleCollaboratorRole(t.Context(), owner, repo, maintainer.ID, repo_model.ArticleCollaboratorRoleMaintainer))
	})

	t.Run("DelegateRemovesMaintainer", func(t *testing.T) {
		require.NoError(t, RemoveArticleCollaborators(t.Context(), delegate, repo, []int64{maintainer.ID}))

		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
		isCollaborator, err := repo_model.IsCollaborator(t.Context(), repo.ID, maintainer.ID)
		require.NoError(t, err)
		assert.False(t, isCollaborator)

		perms, err := CheckForkOnEditPermissions(t.Context(), maintainer, repo)
		require.NoError(t, err)
		assert.False(t, perms.CanEditDirectly)
	})

	t.Run("DeclineRemovesInvite", func(t *testing.T) {
		require.NoError(t, InviteArticleCollaborator(t.Context(), owner, repo, other, repo_model.ArticleCollaboratorRoleMaintainer))
		require.NoError(t, DeclineArticleCollaboratorInvite(t.Context(), other, repo))
		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: other.ID})
	})

	t.Run("SubjectOwnerRefused", func(t *testing.T) {
		// the invitee owns another active article on the subject
		require.NoError(t, db.Insert(t.Context(), &repo_model.Repository{
			OwnerID: other.ID, OwnerName: other.Name, Name: "own-article", LowerName: "own-article", SubjectID: repo.SubjectID,
		}))
		err := InviteArticleCollaborator(t.Context(), owner, repo, other, repo_model.ArticleCollaboratorRoleMaintainer)
		assert.ErrorIs(t, err, ErrArticleCollaboratorOwnsSubject)
	})

	t.Run("SubjectOwnerCannotAccept", func(t *testing.T) {
		// invited first, created their own article afterwards
		require.NoError(t, InviteArticleCollaborator(t.Context(), owner, repo, maintainer, repo_model.ArticleCollaboratorRoleMaintainer))
		require.NoError(t, db.Insert(t.Context(), &repo_model.Repository{
			OwnerID: maintainer.ID, OwnerName: maintainer.Name, Name: "own-article", LowerName: "own-article", SubjectID: repo.SubjectID,
		}))
		assert.ErrorIs(t, AcceptArticleCollaboratorInvite(t.Context(), maintainer, repo), ErrArticleCollaboratorOwnsSubject)

		c := unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: maintainer.ID})
		assert.True(t, c.IsPending())
	})

	t.Run("OwnerRemovesDelegate", func(t *testing.T) {
		require.NoError(t, RemoveArticleCollaborators(t.Context(), owner, repo, []int64{delegate.ID, maintainer.ID}))
		unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID})
		isCollaborator, err := repo_model.IsCollaborator(t.Context(), repo.ID, delegate.ID)
		require.NoError(t, err)
		assert.False(t, isCollaborator)
	})
}
