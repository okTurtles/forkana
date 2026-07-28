// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package user_test

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
)

// TestSearchUsersRepoRole exercises the RepoRole filter added for the explore/users
// "Article owner"/"Contributor"/"Neither" filter. Fixture users 2 and 5 already own a
// root (non-fork, non-empty) repository, so they should count as "owner". User 4 owns no
// repositories at all until this test gives it a fork-only one, so it moves from
// "neither" to "contributor".
func TestSearchUsersRepoRole(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Sanity check the starting state: user 4 owns nothing.
	neitherBefore, _, err := user_model.SearchUsers(t.Context(), user_model.SearchUserOptions{
		Type:     user_model.UserTypeIndividual,
		RepoRole: user_model.RepoRoleNeither,
	})
	assert.NoError(t, err)
	assert.True(t, containsUserID(neitherBefore, 4))

	fork := &repo_model.Repository{
		OwnerID:   4,
		OwnerName: "user4",
		LowerName: "repo-role-test-fork",
		Name:      "repo-role-test-fork",
		IsFork:    true,
		IsEmpty:   false,
	}
	assert.NoError(t, db.Insert(t.Context(), fork))

	owners, _, err := user_model.SearchUsers(t.Context(), user_model.SearchUserOptions{
		Type:     user_model.UserTypeIndividual,
		RepoRole: user_model.RepoRoleOwner,
	})
	assert.NoError(t, err)
	assert.True(t, containsUserID(owners, 2), "user 2 owns a root repo in fixtures")
	assert.True(t, containsUserID(owners, 5), "user 5 owns a root repo in fixtures")
	assert.False(t, containsUserID(owners, 4), "user 4 only owns a fork, not a root repo")

	contributors, _, err := user_model.SearchUsers(t.Context(), user_model.SearchUserOptions{
		Type:     user_model.UserTypeIndividual,
		RepoRole: user_model.RepoRoleContributor,
	})
	assert.NoError(t, err)
	assert.True(t, containsUserID(contributors, 4), "user 4 now owns a fork and no root repo")
	assert.False(t, containsUserID(contributors, 2), "user 2 owns a root repo, so it's an owner, not a contributor")

	neitherAfter, _, err := user_model.SearchUsers(t.Context(), user_model.SearchUserOptions{
		Type:     user_model.UserTypeIndividual,
		RepoRole: user_model.RepoRoleNeither,
	})
	assert.NoError(t, err)
	assert.False(t, containsUserID(neitherAfter, 4), "user 4 now owns a fork, so it's no longer \"neither\"")
	assert.False(t, containsUserID(neitherAfter, 2), "user 2 owns a root repo")
}

func containsUserID(users []*user_model.User, id int64) bool {
	for _, u := range users {
		if u.ID == id {
			return true
		}
	}
	return false
}
