// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"os"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/modules/util"

	"github.com/stretchr/testify/assert"
)

func TestForkRepository(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// user 13 has already forked repo10
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})

	fork, err := ForkRepository(t.Context(), user, user, ForkRepoOptions{
		BaseRepo:    repo,
		Name:        "test",
		Description: "test",
	})
	assert.Nil(t, fork)
	assert.Error(t, err)
	assert.True(t, IsErrForkAlreadyExist(err))

	// user not reached maximum limit of repositories
	assert.False(t, repo_model.IsErrReachLimitOfRepo(err))

	// change AllowForkWithoutMaximumLimit to false for the test
	defer test.MockVariableValue(&setting.Repository.AllowForkWithoutMaximumLimit, false)()
	// user has reached maximum limit of repositories
	user.MaxRepoCreation = 0
	fork2, err := ForkRepository(t.Context(), user, user, ForkRepoOptions{
		BaseRepo:    repo,
		Name:        "test",
		Description: "test",
	})
	assert.Nil(t, fork2)
	assert.True(t, repo_model.IsErrReachLimitOfRepo(err))
}

func TestForkRepositoryCleanup(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// a successful fork
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo10 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})

	fork, err := ForkRepository(t.Context(), user2, user2, ForkRepoOptions{
		BaseRepo: repo10,
		Name:     "test",
	})
	assert.NoError(t, err)
	assert.NotNil(t, fork)

	exist, err := util.IsExist(repo_model.RepoPath(user2.Name, "test"))
	assert.NoError(t, err)
	assert.True(t, exist)

	err = DeleteRepositoryDirectly(t.Context(), fork.ID)
	assert.NoError(t, err)

	// a failed creating because some mock data
	// create the repository directory so that the creation will fail after database record created.
	assert.NoError(t, os.MkdirAll(repo_model.RepoPath(user2.Name, "test"), os.ModePerm))

	fork2, err := ForkRepository(t.Context(), user2, user2, ForkRepoOptions{
		BaseRepo: repo10,
		Name:     "test",
	})
	assert.Nil(t, fork2)
	assert.Error(t, err)

	// assert the cleanup is successful
	unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerName: user2.Name, Name: "test"})

	exist, err = util.IsExist(repo_model.RepoPath(user2.Name, "test"))
	assert.NoError(t, err)
	assert.False(t, exist)
}

func TestConvertNormalToForkRepository(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	t.Run("ConvertNormalToFork", func(t *testing.T) {
		assert.NoError(t, unittest.PrepareTestDatabase())

		// Create a root repository (non-empty, non-fork)
		rootRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
			Name: "convert-test-root",
		}, true)
		assert.NoError(t, err)
		assert.NotNil(t, rootRepo)
		rootRepo.IsEmpty = false
		assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "is_empty"))

		// Create a normal repository that will be converted to a fork
		normalRepo, err := CreateRepositoryDirectly(t.Context(), user4, user4, CreateRepoOptions{
			Name: "convert-test-normal",
		}, true)
		assert.NoError(t, err)
		assert.NotNil(t, normalRepo)
		assert.False(t, normalRepo.IsFork)
		assert.Equal(t, int64(0), normalRepo.ForkID)

		// Get initial fork count on root
		rootRepo, err = repo_model.GetRepositoryByID(t.Context(), rootRepo.ID)
		assert.NoError(t, err)
		initialForkCount := rootRepo.NumForks

		// Convert normal repo to fork
		err = ConvertNormalToForkRepository(t.Context(), normalRepo, rootRepo.ID)
		assert.NoError(t, err)

		// Verify the repository is now a fork
		convertedRepo, err := repo_model.GetRepositoryByID(t.Context(), normalRepo.ID)
		assert.NoError(t, err)
		assert.True(t, convertedRepo.IsFork)
		assert.Equal(t, rootRepo.ID, convertedRepo.ForkID)

		// Verify fork count was incremented
		rootRepo, err = repo_model.GetRepositoryByID(t.Context(), rootRepo.ID)
		assert.NoError(t, err)
		assert.Equal(t, initialForkCount+1, rootRepo.NumForks)

		// Cleanup
		_ = DeleteRepositoryDirectly(t.Context(), normalRepo.ID)
		_ = DeleteRepositoryDirectly(t.Context(), rootRepo.ID)
	})

	t.Run("IdempotentAlreadyFork", func(t *testing.T) {
		assert.NoError(t, unittest.PrepareTestDatabase())

		// Create a root repository
		rootRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
			Name: "idempotent-test-root",
		}, true)
		assert.NoError(t, err)
		rootRepo.IsEmpty = false
		assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "is_empty"))

		// Create a normal repository and convert it to a fork
		normalRepo, err := CreateRepositoryDirectly(t.Context(), user4, user4, CreateRepoOptions{
			Name: "idempotent-test-normal",
		}, true)
		assert.NoError(t, err)

		// First conversion
		err = ConvertNormalToForkRepository(t.Context(), normalRepo, rootRepo.ID)
		assert.NoError(t, err)

		// Get fork count after first conversion
		rootRepo, err = repo_model.GetRepositoryByID(t.Context(), rootRepo.ID)
		assert.NoError(t, err)
		forkCountAfterFirst := rootRepo.NumForks

		// Second conversion (should be no-op)
		err = ConvertNormalToForkRepository(t.Context(), normalRepo, rootRepo.ID)
		assert.NoError(t, err)

		// Verify fork count was NOT incremented again
		rootRepo, err = repo_model.GetRepositoryByID(t.Context(), rootRepo.ID)
		assert.NoError(t, err)
		assert.Equal(t, forkCountAfterFirst, rootRepo.NumForks)

		// Cleanup
		_ = DeleteRepositoryDirectly(t.Context(), normalRepo.ID)
		_ = DeleteRepositoryDirectly(t.Context(), rootRepo.ID)
	})

	t.Run("SelfForkPrevention", func(t *testing.T) {
		assert.NoError(t, unittest.PrepareTestDatabase())

		// Create a repository
		repo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
			Name: "self-fork-test",
		}, true)
		assert.NoError(t, err)
		assert.False(t, repo.IsFork)

		// Try to convert to fork of itself (should be no-op)
		err = ConvertNormalToForkRepository(t.Context(), repo, repo.ID)
		assert.NoError(t, err)

		// Verify it's still not a fork
		repo, err = repo_model.GetRepositoryByID(t.Context(), repo.ID)
		assert.NoError(t, err)
		assert.False(t, repo.IsFork)
		assert.Equal(t, int64(0), repo.ForkID)

		// Cleanup
		_ = DeleteRepositoryDirectly(t.Context(), repo.ID)
	})

	t.Run("NonExistentRootRepo", func(t *testing.T) {
		assert.NoError(t, unittest.PrepareTestDatabase())

		// Create a normal repository
		normalRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
			Name: "nonexistent-root-test",
		}, true)
		assert.NoError(t, err)

		// Try to convert to fork of non-existent repository
		// This should fail because the root repository doesn't exist
		err = ConvertNormalToForkRepository(t.Context(), normalRepo, 999999)
		assert.Error(t, err)
		assert.True(t, repo_model.IsErrRepoNotExist(err))

		// Verify the repo was NOT converted (still not a fork)
		unconvertedRepo, err := repo_model.GetRepositoryByID(t.Context(), normalRepo.ID)
		assert.NoError(t, err)
		assert.False(t, unconvertedRepo.IsFork)
		assert.Equal(t, int64(0), unconvertedRepo.ForkID)

		// Cleanup
		_ = DeleteRepositoryDirectly(t.Context(), normalRepo.ID)
	})

	t.Run("ForkTreeLimitEnforced", func(t *testing.T) {
		assert.NoError(t, unittest.PrepareTestDatabase())

		// Save original setting
		originalLimit := setting.Repository.MaxForkTreeNodes
		defer func() {
			setting.Repository.MaxForkTreeNodes = originalLimit
		}()

		// Create a root repository
		rootRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
			Name: "fork-limit-test-root",
		}, true)
		assert.NoError(t, err)
		rootRepo.IsEmpty = false
		assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "is_empty"))

		// Create a normal repository to convert
		normalRepo, err := CreateRepositoryDirectly(t.Context(), user4, user4, CreateRepoOptions{
			Name: "fork-limit-test-normal",
		}, true)
		assert.NoError(t, err)
		assert.False(t, normalRepo.IsFork)

		// Set limit to 0 (prevent all forking)
		setting.Repository.MaxForkTreeNodes = 0
		err = ConvertNormalToForkRepository(t.Context(), normalRepo, rootRepo.ID)
		assert.Error(t, err)
		assert.True(t, repo_model.IsErrForkTreeTooLarge(err))

		// Verify the repo was NOT converted
		unconvertedRepo, err := repo_model.GetRepositoryByID(t.Context(), normalRepo.ID)
		assert.NoError(t, err)
		assert.False(t, unconvertedRepo.IsFork)

		// Set limit to 1 (only root allowed) - should also fail
		setting.Repository.MaxForkTreeNodes = 1
		err = ConvertNormalToForkRepository(t.Context(), normalRepo, rootRepo.ID)
		assert.Error(t, err)
		assert.True(t, repo_model.IsErrForkTreeTooLarge(err))

		// Set limit to -1 (disabled) - should succeed
		setting.Repository.MaxForkTreeNodes = -1
		err = ConvertNormalToForkRepository(t.Context(), normalRepo, rootRepo.ID)
		assert.NoError(t, err)

		// Verify the repo was converted
		convertedRepo, err := repo_model.GetRepositoryByID(t.Context(), normalRepo.ID)
		assert.NoError(t, err)
		assert.True(t, convertedRepo.IsFork)
		assert.Equal(t, rootRepo.ID, convertedRepo.ForkID)

		// Cleanup
		_ = DeleteRepositoryDirectly(t.Context(), normalRepo.ID)
		_ = DeleteRepositoryDirectly(t.Context(), rootRepo.ID)
	})
}

func TestForkRepositoryTreeSizeLimit(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo10 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})

	// Save original setting
	originalLimit := setting.Repository.MaxForkTreeNodes
	defer func() {
		setting.Repository.MaxForkTreeNodes = originalLimit
	}()

	// Test with limit disabled (-1) - should succeed
	setting.Repository.MaxForkTreeNodes = -1
	fork1, err := ForkRepository(t.Context(), user2, user2, ForkRepoOptions{
		BaseRepo: repo10,
		Name:     "test-unlimited",
	})
	if fork1 != nil {
		defer func() {
			_ = DeleteRepositoryDirectly(t.Context(), fork1.ID)
		}()
	}
	assert.NoError(t, err)
	assert.NotNil(t, fork1)

	// Clean up
	if fork1 != nil {
		err = DeleteRepositoryDirectly(t.Context(), fork1.ID)
		assert.NoError(t, err)
	}

	// Test with limit = 0 - should fail
	setting.Repository.MaxForkTreeNodes = 0
	fork2, err := ForkRepository(t.Context(), user2, user2, ForkRepoOptions{
		BaseRepo: repo10,
		Name:     "test-zero-limit",
	})
	assert.Nil(t, fork2)
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrForkTreeTooLarge(err))

	// Test with limit = 1 (only root allowed) - should fail
	setting.Repository.MaxForkTreeNodes = 1
	fork3, err := ForkRepository(t.Context(), user2, user2, ForkRepoOptions{
		BaseRepo: repo10,
		Name:     "test-one-limit",
	})
	assert.Nil(t, fork3)
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrForkTreeTooLarge(err))

	// Test with high limit - should succeed
	setting.Repository.MaxForkTreeNodes = 1000
	fork4, err := ForkRepository(t.Context(), user2, user2, ForkRepoOptions{
		BaseRepo: repo10,
		Name:     "test-high-limit",
	})
	if fork4 != nil {
		defer func() {
			_ = DeleteRepositoryDirectly(t.Context(), fork4.ID)
		}()
	}
	assert.NoError(t, err)
	assert.NotNil(t, fork4)

	// Clean up
	if fork4 != nil {
		err = DeleteRepositoryDirectly(t.Context(), fork4.ID)
		assert.NoError(t, err)
	}
}

// TestCheckForkOnEditPermissions tests the CheckForkOnEditPermissions function
// which determines how a user can edit a repository they don't own.
func TestCheckForkOnEditPermissions(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	t.Run("RepoOwner", func(t *testing.T) {
		// User owns the repository - should be able to edit directly
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		perms, err := CheckForkOnEditPermissions(t.Context(), user, repo)
		assert.NoError(t, err)
		assert.True(t, perms.IsRepoOwner)
		assert.True(t, perms.CanEditDirectly)
		assert.False(t, perms.NeedsFork)
		assert.False(t, perms.HasExistingFork)
		assert.False(t, perms.BlockedBySubject)
		assert.False(t, perms.CanSubmitChangeRequest)
	})

	t.Run("NonOwnerNeedsFork", func(t *testing.T) {
		// User doesn't own the repository and has no fork - should need to fork
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		perms, err := CheckForkOnEditPermissions(t.Context(), user, repo)
		assert.NoError(t, err)
		assert.False(t, perms.IsRepoOwner)
		assert.False(t, perms.CanEditDirectly)
		assert.True(t, perms.NeedsFork)
		assert.False(t, perms.HasExistingFork)
		assert.False(t, perms.BlockedBySubject)
		assert.True(t, perms.CanSubmitChangeRequest)
	})

	t.Run("UserWithExistingFork", func(t *testing.T) {
		// User has an existing fork of the repository
		// repo11 is a fork of repo10 owned by user13
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
		baseRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})

		perms, err := CheckForkOnEditPermissions(t.Context(), user, baseRepo)
		assert.NoError(t, err)
		assert.False(t, perms.IsRepoOwner)
		assert.False(t, perms.CanEditDirectly)
		assert.False(t, perms.NeedsFork)
		assert.True(t, perms.HasExistingFork)
		assert.False(t, perms.BlockedBySubject)
		assert.True(t, perms.CanSubmitChangeRequest)
		assert.NotNil(t, perms.ExistingFork)
		assert.Equal(t, int64(11), perms.ExistingFork.ID)
	})

	t.Run("AnonymousUserNoPermissions", func(t *testing.T) {
		// Anonymous user (nil doer) should have no permissions
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		perms, err := CheckForkOnEditPermissions(t.Context(), nil, repo)
		assert.NoError(t, err)
		assert.False(t, perms.IsRepoOwner)
		assert.False(t, perms.CanEditDirectly)
		assert.False(t, perms.NeedsFork)
		assert.False(t, perms.HasExistingFork)
		assert.False(t, perms.BlockedBySubject)
		assert.False(t, perms.CanSubmitChangeRequest)
	})
}
