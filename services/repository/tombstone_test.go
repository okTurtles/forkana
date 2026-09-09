// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository_test

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	repo_service "code.gitea.io/gitea/services/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanBeTombstoneDeleted(t *testing.T) {
	unittest.PrepareTestEnv(t)

	cases := []struct {
		name     string
		repoID   int64
		expected bool
	}{
		// repo 10 is the base of repo 11 in the fixtures.
		{name: "WithForks", repoID: 10, expected: true},
		{name: "WithoutForks", repoID: 11, expected: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: c.repoID})
			got, err := repo_service.CanBeTombstoneDeleted(t.Context(), repo)
			require.NoError(t, err)
			assert.Equal(t, c.expected, got)
		})
	}
}

func TestTombstoneRepository(t *testing.T) {
	unittest.PrepareTestEnv(t)

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	require.NoError(t, repo_service.TombstoneRepository(t.Context(), repo))

	assert.True(t, repo.IsTombstone())
	assert.False(t, repo.TombstonedUnix.IsZero())

	stored := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	assert.True(t, stored.IsTombstone())
	assert.Equal(t, repo.TombstonedUnix, stored.TombstonedUnix)

	// A tombstone cannot be tombstoned again, so it is never reported as deletable.
	deletable, err := repo_service.CanBeTombstoneDeleted(t.Context(), stored)
	require.NoError(t, err)
	assert.False(t, deletable)

	// Tombstoning is idempotent.
	require.NoError(t, repo_service.TombstoneRepository(t.Context(), stored))
}

func TestDeleteRepositoryTombstonesWhenForked(t *testing.T) {
	unittest.PrepareTestEnv(t)

	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})

	tombstoned, err := repo_service.DeleteRepository(t.Context(), doer, repo, false)
	require.NoError(t, err)
	assert.True(t, tombstoned)

	// The row and the fork relation are kept so that the fork retains its ancestor.
	stored := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	assert.True(t, stored.IsTombstone())
	unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11, ForkID: 10})
}

func TestSearchRepositoryExcludesTombstones(t *testing.T) {
	unittest.PrepareTestEnv(t)

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	require.NoError(t, repo_service.TombstoneRepository(t.Context(), repo))

	search := func(includeTombstoned bool) bool {
		repos, _, err := repo_model.SearchRepository(t.Context(), repo_model.SearchRepoOptions{
			ListOptions:       db.ListOptionsAll,
			Private:           true,
			IncludeTombstoned: includeTombstoned,
		})
		require.NoError(t, err)
		for _, r := range repos {
			if r.ID == repo.ID {
				return true
			}
		}
		return false
	}

	assert.False(t, search(false))
	assert.True(t, search(true))
}
