// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	repo_service "code.gitea.io/gitea/services/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanBlockUser(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user1 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	user29 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 29})
	org3 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 3})

	// Doer can't self block
	assert.False(t, CanBlockUser(t.Context(), user1, user2, user1))
	// Blocker can't be blockee
	assert.False(t, CanBlockUser(t.Context(), user1, user2, user2))
	// Can't block already blocked user
	assert.False(t, CanBlockUser(t.Context(), user1, user2, user29))
	// Blockee can't be an organization
	assert.False(t, CanBlockUser(t.Context(), user1, user2, org3))
	// Doer must be blocker or admin
	assert.False(t, CanBlockUser(t.Context(), user2, user4, user29))
	// Organization can't block a member
	assert.False(t, CanBlockUser(t.Context(), user1, org3, user4))
	// Doer must be organization owner or admin if blocker is an organization
	assert.False(t, CanBlockUser(t.Context(), user4, org3, user2))

	assert.True(t, CanBlockUser(t.Context(), user1, user2, user4))
	assert.True(t, CanBlockUser(t.Context(), user2, user2, user4))
	assert.True(t, CanBlockUser(t.Context(), user2, org3, user29))
}

func TestCanUnblockUser(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user1 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user28 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 28})
	user29 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 29})
	org17 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 17})

	// Doer can't self unblock
	assert.False(t, CanUnblockUser(t.Context(), user1, user2, user1))
	// Can't unblock not blocked user
	assert.False(t, CanUnblockUser(t.Context(), user1, user2, user28))
	// Doer must be blocker or admin
	assert.False(t, CanUnblockUser(t.Context(), user28, user2, user29))
	// Doer must be organization owner or admin if blocker is an organization
	assert.False(t, CanUnblockUser(t.Context(), user2, org17, user28))

	assert.True(t, CanUnblockUser(t.Context(), user1, user2, user29))
	assert.True(t, CanUnblockUser(t.Context(), user2, user2, user29))
	assert.True(t, CanUnblockUser(t.Context(), user1, org17, user28))
}

// TestBlockUserRemovesArticleCollaborators checks that blocking removes the blockee from the
// collaborators of the blocker's articles, accepted or invited: the row alone keeps the
// delegate and maintainer rights.
func TestBlockUserRemovesArticleCollaborators(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	// repo1 is an article owned by user2
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	delegate := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	invitee := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	require.NoError(t, repo_service.InviteArticleCollaborator(t.Context(), owner, repo, delegate, repo_model.ArticleCollaboratorRoleMaintainerDelegate))
	require.NoError(t, repo_service.AcceptArticleCollaboratorInvite(t.Context(), delegate, repo))
	require.NoError(t, repo_service.InviteArticleCollaborator(t.Context(), owner, repo, invitee, repo_model.ArticleCollaboratorRoleMaintainer))

	require.NoError(t, BlockUser(t.Context(), owner, owner, delegate, ""))
	unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: delegate.ID})
	p, err := repo_service.GetArticleCollaboratorPermission(t.Context(), delegate, repo)
	require.NoError(t, err)
	assert.False(t, p.CanManage)

	// blocking the owner works the other way round too
	require.NoError(t, BlockUser(t.Context(), invitee, invitee, owner, ""))
	unittest.AssertNotExistsBean(t, &repo_model.ArticleCollaborator{RepoID: repo.ID, UserID: invitee.ID})
}
