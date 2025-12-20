// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"

	"github.com/stretchr/testify/assert"
)

func TestEditorUtils(t *testing.T) {
	unittest.PrepareTestEnv(t)
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	t.Run("getUniquePatchBranchName", func(t *testing.T) {
		branchName := getUniquePatchBranchName(t.Context(), "user2", repo)
		assert.Equal(t, "user2-patch-1", branchName)
	})
	t.Run("getClosestParentWithFiles", func(t *testing.T) {
		gitRepo, _ := gitrepo.OpenRepository(t.Context(), repo)
		defer gitRepo.Close()
		treePath := getClosestParentWithFiles(gitRepo, "sub-home-md-img-check", "docs/foo/bar")
		assert.Equal(t, "docs", treePath)
		treePath = getClosestParentWithFiles(gitRepo, "sub-home-md-img-check", "any/other")
		assert.Empty(t, treePath)
	})
}

func TestGetUniqueRepositoryName(t *testing.T) {
	unittest.PrepareTestEnv(t)

	// user2 owns repo1 (name: "repo1")
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	t.Run("returns base name when not taken", func(t *testing.T) {
		// "unique-test-repo" doesn't exist for user2
		name := getUniqueRepositoryName(t.Context(), user2.ID, "unique-test-repo")
		assert.Equal(t, "unique-test-repo", name)
	})

	t.Run("returns name with suffix when base name is taken", func(t *testing.T) {
		// user2 owns "repo1", so asking for "repo1" should return "repo1-1"
		name := getUniqueRepositoryName(t.Context(), user2.ID, "repo1")
		assert.Equal(t, "repo1-1", name)
	})

	t.Run("case insensitive name matching", func(t *testing.T) {
		// user2 owns "repo1", asking for "REPO1" should also return with suffix
		name := getUniqueRepositoryName(t.Context(), user2.ID, "REPO1")
		assert.Equal(t, "REPO1-1", name)
	})

	t.Run("returns base name for different owner", func(t *testing.T) {
		// user5 doesn't own "repo1", so should get the base name
		user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
		name := getUniqueRepositoryName(t.Context(), user5.ID, "repo1")
		assert.Equal(t, "repo1", name)
	})
}
