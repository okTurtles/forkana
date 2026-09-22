// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoAssignees(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	users, err := repo_model.GetRepoAssignees(t.Context(), repo2)
	assert.NoError(t, err)
	assert.Len(t, users, 1)
	assert.Equal(t, int64(2), users[0].ID)

	repo21 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 21})
	users, err = repo_model.GetRepoAssignees(t.Context(), repo21)
	assert.NoError(t, err)
	if assert.Len(t, users, 4) {
		assert.ElementsMatch(t, []int64{10, 15, 16, 18}, []int64{users[0].ID, users[1].ID, users[2].ID, users[3].ID})
	}

	// do not return deactivated users
	assert.NoError(t, user_model.UpdateUserCols(t.Context(), &user_model.User{ID: 15, IsActive: false}, "is_active"))
	users, err = repo_model.GetRepoAssignees(t.Context(), repo21)
	assert.NoError(t, err)
	if assert.Len(t, users, 3) {
		assert.NotContains(t, []int64{users[0].ID, users[1].ID, users[2].ID}, 15)
	}
}

func TestGetStarredReposExcludesTombstones(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// user2 starred repo2 and repo4.
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(),
		&repo_model.Repository{ID: 4, IsTombstoned: true}, "is_tombstoned"))

	opts := &repo_model.StarredReposOptions{StarrerID: 2, IncludePrivate: true}
	repos, err := repo_model.GetStarredRepos(t.Context(), opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{2}, repoIDsOf(repos))

	// The rows are still needed by the callers that clean them up.
	opts.IncludeTombstoned = true
	repos, err = repo_model.GetStarredRepos(t.Context(), opts)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{2, 4}, repoIDsOf(repos))
}

func TestGetWatchedReposExcludesTombstones(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// user1 watches repo1 only.
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(),
		&repo_model.Repository{ID: 1, IsTombstoned: true}, "is_tombstoned"))

	opts := &repo_model.WatchedReposOptions{WatcherID: 1, IncludePrivate: true}
	repos, count, err := repo_model.GetWatchedRepos(t.Context(), opts)
	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Empty(t, repos)

	opts.IncludeTombstoned = true
	repos, count, err = repo_model.GetWatchedRepos(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.ElementsMatch(t, []int64{1}, repoIDsOf(repos))
}

func repoIDsOf(repos []*repo_model.Repository) []int64 {
	ids := make([]int64, 0, len(repos))
	for _, repo := range repos {
		ids = append(ids, repo.ID)
	}
	return ids
}

func TestGetIssuePostersWithSearch(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})

	users, err := repo_model.GetIssuePostersWithSearch(t.Context(), repo2, false, "USER", false /* full name */)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "user2", users[0].Name)

	users, err = repo_model.GetIssuePostersWithSearch(t.Context(), repo2, false, "TW%O", true /* full name */)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "user2", users[0].Name)
}
