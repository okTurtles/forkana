// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTeamRepositoriesExcludesTombstones(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// repo5 belongs to team 1 of org 3.
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(),
		&repo_model.Repository{ID: 5, IsTombstoned: true}, "is_tombstoned"))

	opts := &repo_model.SearchTeamRepoOptions{TeamID: 1}
	repos, err := repo_model.GetTeamRepositories(t.Context(), opts)
	require.NoError(t, err)
	assert.NotContains(t, repoIDsOf(repos), int64(5))
	assert.Contains(t, repoIDsOf(repos), int64(3))

	// Access and watch maintenance still has to see the tombstone.
	opts.IncludeTombstoned = true
	repos, err = repo_model.GetTeamRepositories(t.Context(), opts)
	require.NoError(t, err)
	assert.Contains(t, repoIDsOf(repos), int64(5))
}
