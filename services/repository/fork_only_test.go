// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserAcceptsChangeRequests(t *testing.T) {
	assert.False(t, UserAcceptsChangeRequests(nil))
	assert.True(t, UserAcceptsChangeRequests(&user_model.User{IsActive: true}))
	assert.False(t, UserAcceptsChangeRequests(&user_model.User{IsActive: true, IsForkOnly: true}))
	assert.False(t, UserAcceptsChangeRequests(&user_model.User{IsActive: false}), "a deactivated user")
	assert.False(t, UserAcceptsChangeRequests(&user_model.User{IsActive: true, ProhibitLogin: true}), "a user who cannot sign in")
	assert.False(t, UserAcceptsChangeRequests(user_model.NewGhostUser()), "a deleted user")
	assert.True(t, UserAcceptsChangeRequests(&user_model.User{Type: user_model.UserTypeOrganization}), "an organization's members review for it")
	assert.False(t, UserAcceptsChangeRequests(&user_model.User{Type: user_model.UserTypeOrganization, IsForkOnly: true}))
}

// TestIsArticleForkOnly checks when nobody can review the change requests of an article:
// its owner does not accept them, and it has no active maintainer.
func TestIsArticleForkOnly(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// repo1 (user2) is an article of subject 1
	repoID := int64(1)
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	maintainer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	editor := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	setUser := func(t *testing.T, u *user_model.User, cols ...string) {
		_, err := db.GetEngine(ctx).ID(u.ID).Cols(cols...).Update(u)
		require.NoError(t, err)
	}
	isForkOnly := func(t *testing.T) bool {
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		require.NotZero(t, repo.SubjectID)
		forkOnly, gotOwner, err := IsArticleForkOnly(ctx, repo)
		require.NoError(t, err)
		require.NotNil(t, gotOwner)
		assert.Equal(t, owner.ID, gotOwner.ID)
		return forkOnly
	}
	addMaintainer := func(t *testing.T, u *user_model.User, status repo_model.ArticleCollaboratorStatus) {
		require.NoError(t, repo_model.CreateArticleCollaborator(ctx, &repo_model.ArticleCollaborator{
			RepoID: repoID, UserID: u.ID, InviterID: owner.ID,
			Role: repo_model.ArticleCollaboratorRoleMaintainer, Status: status,
		}))
	}

	t.Run("OwnerAcceptsChangeRequests", func(t *testing.T) {
		assert.False(t, isForkOnly(t))
	})

	t.Run("ForkOnlyOwner", func(t *testing.T) {
		owner.IsForkOnly = true
		setUser(t, owner, "is_fork_only")
		defer func() {
			owner.IsForkOnly = false
			setUser(t, owner, "is_fork_only")
		}()
		assert.True(t, isForkOnly(t))

		// a pending invitee cannot review yet
		addMaintainer(t, maintainer, repo_model.ArticleCollaboratorStatusPending)
		assert.True(t, isForkOnly(t))

		// an accepted maintainer can
		c, err := repo_model.GetArticleCollaborator(ctx, repoID, maintainer.ID)
		require.NoError(t, err)
		c.Status = repo_model.ArticleCollaboratorStatusAccepted
		require.NoError(t, repo_model.UpdateArticleCollaboratorCols(ctx, c, "status"))
		assert.False(t, isForkOnly(t))

		// unless they cannot sign in, are deactivated or fork-only themselves
		for _, col := range []string{"prohibit_login", "is_active", "is_fork_only"} {
			saved := *maintainer
			switch col {
			case "prohibit_login":
				maintainer.ProhibitLogin = true
			case "is_active":
				maintainer.IsActive = false
			case "is_fork_only":
				maintainer.IsForkOnly = true
			}
			setUser(t, maintainer, col)
			assert.True(t, isForkOnly(t), "a maintainer with %s changed does not review", col)
			*maintainer = saved
			setUser(t, maintainer, col)
		}
		assert.False(t, isForkOnly(t))

		require.NoError(t, repo_model.DeleteArticleCollaborator(ctx, repoID, maintainer.ID))
		assert.True(t, isForkOnly(t))
	})

	t.Run("DeactivatedOwner", func(t *testing.T) {
		owner.IsActive = false
		setUser(t, owner, "is_active")
		defer func() {
			owner.IsActive = true
			setUser(t, owner, "is_active")
		}()
		assert.True(t, isForkOnly(t))
	})

	t.Run("OwnerProhibitedFromSigningIn", func(t *testing.T) {
		owner.ProhibitLogin = true
		setUser(t, owner, "prohibit_login")
		defer func() {
			owner.ProhibitLogin = false
			setUser(t, owner, "prohibit_login")
		}()
		assert.True(t, isForkOnly(t))

		// the permissions turn the change request into a fork, for a user who could submit one
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		perms, err := CheckForkOnEditPermissions(ctx, editor, repo)
		require.NoError(t, err)
		assert.True(t, perms.ForkOnly)
		assert.Equal(t, owner.Name, perms.ForkOnlyOwnerName)
		assert.False(t, perms.CanSubmitChangeRequest)
		assert.True(t, perms.NeedsFork)

		// and the owner still edits directly and may open change requests
		perms, err = CheckForkOnEditPermissions(ctx, owner, repo)
		require.NoError(t, err)
		assert.True(t, perms.CanEditDirectly)
		assert.False(t, perms.ForkOnly)
		require.NoError(t, CheckArticleAcceptsChangeRequest(ctx, owner, repo))

		assert.ErrorIs(t, CheckArticleAcceptsChangeRequest(ctx, editor, repo), ErrArticleForkOnly)
	})

	t.Run("RepositoryWithoutSubject", func(t *testing.T) {
		owner.IsForkOnly = true
		setUser(t, owner, "is_fork_only")
		defer func() {
			owner.IsForkOnly = false
			setUser(t, owner, "is_fork_only")
		}()
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		repo.SubjectID = 0
		forkOnly, _, err := IsArticleForkOnly(ctx, repo)
		require.NoError(t, err)
		assert.False(t, forkOnly, "a repository without a subject is not an article")
	})

	t.Run("EditorCanSubmitChangeRequests", func(t *testing.T) {
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
		perms, err := CheckForkOnEditPermissions(ctx, editor, repo)
		require.NoError(t, err)
		assert.False(t, perms.ForkOnly)
		assert.True(t, perms.CanSubmitChangeRequest)
		require.NoError(t, CheckArticleAcceptsChangeRequest(ctx, editor, repo))
	})
}
