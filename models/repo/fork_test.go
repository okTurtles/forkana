// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"strings"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserFork(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// User13 has repo 11 forked from repo10
	repo, err := repo_model.GetRepositoryByID(t.Context(), 10)
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	repo, err = repo_model.GetUserFork(t.Context(), repo.ID, 13)
	assert.NoError(t, err)
	assert.NotNil(t, repo)

	repo, err = repo_model.GetRepositoryByID(t.Context(), 9)
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	repo, err = repo_model.GetUserFork(t.Context(), repo.ID, 13)
	assert.NoError(t, err)
	assert.Nil(t, repo)
}

func TestForkAncestorIDs(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// repo11 is a fork of repo10 in the fixtures; the chain is extended here.
	newFork := func(name string, parentID int64) *repo_model.Repository {
		repo := &repo_model.Repository{
			OwnerID:   2,
			OwnerName: "user2",
			Name:      name,
			LowerName: strings.ToLower(name),
			IsFork:    true,
			ForkID:    parentID,
		}
		require.NoError(t, db.Insert(t.Context(), repo))
		return repo
	}
	grandchild := newFork("fork-ancestors-grandchild", 11)

	ancestorIDs, err := repo_model.ForkAncestorIDs(t.Context(), grandchild.ID)
	assert.NoError(t, err)
	assert.Equal(t, []int64{11, 10}, ancestorIDs)

	// a repository that is not a fork has no ancestors, and neither has none
	ancestorIDs, err = repo_model.ForkAncestorIDs(t.Context(), 10)
	assert.NoError(t, err)
	assert.Empty(t, ancestorIDs)

	ancestorIDs, err = repo_model.ForkAncestorIDs(t.Context(), 0)
	assert.NoError(t, err)
	assert.Empty(t, ancestorIDs)

	// the walk stops where the fork tree limit stops
	t.Run("BoundedByTheForkTreeLimit", func(t *testing.T) {
		defer test.MockVariableValue(&setting.Repository.MaxForkTreeNodes, 2)()

		ancestorIDs, err := repo_model.ForkAncestorIDs(t.Context(), grandchild.ID)
		assert.NoError(t, err)
		assert.Equal(t, []int64{11}, ancestorIDs)
	})
}
