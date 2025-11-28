// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"os"
	"sync"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRepositoryDirectly(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// a successful creating repository
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	createdRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name: "created-repo",
	}, true)
	assert.NoError(t, err)
	assert.NotNil(t, createdRepo)

	exist, err := util.IsExist(repo_model.RepoPath(user2.Name, createdRepo.Name))
	assert.NoError(t, err)
	assert.True(t, exist)

	unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: user2.Name, Name: createdRepo.Name})

	err = DeleteRepositoryDirectly(t.Context(), createdRepo.ID)
	assert.NoError(t, err)

	// a failed creating because some mock data
	// create the repository directory so that the creation will fail after database record created.
	assert.NoError(t, os.MkdirAll(repo_model.RepoPath(user2.Name, createdRepo.Name), os.ModePerm))

	createdRepo2, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name: "created-repo",
	}, true)
	assert.Nil(t, createdRepo2)
	assert.Error(t, err)

	// assert the cleanup is successful
	unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerName: user2.Name, Name: createdRepo.Name})

	exist, err = util.IsExist(repo_model.RepoPath(user2.Name, createdRepo.Name))
	assert.NoError(t, err)
	assert.False(t, exist)
}

func TestFirstArticleBecomesRoot(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Get two different users
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	subjectName := "test-first-article-subject"

	// User 2 creates the first article for this subject - should become root
	rootRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:    "first-article",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, rootRepo)

	// Verify it's a root repository (not a fork)
	assert.False(t, rootRepo.IsFork)
	assert.Equal(t, int64(0), rootRepo.ForkID)
	assert.Positive(t, rootRepo.SubjectID)

	// User 4 creates an article for the same subject - should become a fork
	forkRepo, err := CreateRepositoryDirectly(t.Context(), user4, user4, CreateRepoOptions{
		Name:    "second-article",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, forkRepo)

	// Verify it's a fork of the root repository
	assert.True(t, forkRepo.IsFork)
	assert.Equal(t, rootRepo.ID, forkRepo.ForkID)
	assert.Equal(t, rootRepo.SubjectID, forkRepo.SubjectID)

	// Cleanup
	err = DeleteRepositoryDirectly(t.Context(), forkRepo.ID)
	assert.NoError(t, err)
	err = DeleteRepositoryDirectly(t.Context(), rootRepo.ID)
	assert.NoError(t, err)
}

func TestFirstArticleBecomesRoot_SameUserSecondArticle(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	subjectName := "test-same-user-subject"

	// User 2 creates the first article - should become root
	rootRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:    "first-article",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, rootRepo)
	assert.False(t, rootRepo.IsFork)

	// Same user tries to create another article for the same subject
	// This creates a fork of their own root repository
	// Note: This is currently allowed - the user can have both a root and a fork
	// If we want to prevent this, we need to add a check for existing repos with the same subject
	forkRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:    "second-article",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, forkRepo)

	// The second repo is a fork of the first
	assert.True(t, forkRepo.IsFork)
	assert.Equal(t, rootRepo.ID, forkRepo.ForkID)
	assert.Equal(t, rootRepo.SubjectID, forkRepo.SubjectID)

	// Cleanup
	err = DeleteRepositoryDirectly(t.Context(), forkRepo.ID)
	assert.NoError(t, err)
	err = DeleteRepositoryDirectly(t.Context(), rootRepo.ID)
	assert.NoError(t, err)
}

func TestFirstArticleBecomesRoot_NoSubject(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	// Create repository without subject - should work normally
	repo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name: "no-subject-repo",
	}, true)
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Verify it's a root repository with no subject
	assert.False(t, repo.IsFork)
	assert.Equal(t, int64(0), repo.SubjectID)

	// Cleanup
	err = DeleteRepositoryDirectly(t.Context(), repo.ID)
	assert.NoError(t, err)
}

func TestFirstArticleBecomesRoot_ConcurrentCreation(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Get multiple users for concurrent creation
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	subjectName := "test-concurrent-subject"

	var wg sync.WaitGroup
	var mu sync.Mutex
	var repos []*repo_model.Repository
	var errors []error

	// Create articles concurrently from 3 users
	users := []*user_model.User{user2, user4, user5}
	for i, user := range users {
		wg.Add(1)
		go func(u *user_model.User, idx int) {
			defer wg.Done()
			repo, err := CreateRepositoryDirectly(t.Context(), u, u, CreateRepoOptions{
				Name:    "concurrent-article",
				Subject: subjectName,
			}, true)
			mu.Lock()
			repos = append(repos, repo)
			errors = append(errors, err)
			mu.Unlock()
		}(user, i)
	}

	wg.Wait()

	// Count successful creations and forks
	var rootCount, forkCount int
	for i, repo := range repos {
		if errors[i] != nil {
			continue
		}
		if repo.IsFork {
			forkCount++
		} else {
			rootCount++
		}
	}

	// Exactly one should be root, others should be forks
	assert.Equal(t, 1, rootCount, "Expected exactly one root repository")
	assert.Equal(t, 2, forkCount, "Expected two fork repositories")

	// All forks should point to the same root
	var rootID int64
	for i, repo := range repos {
		if errors[i] != nil {
			continue
		}
		if !repo.IsFork {
			rootID = repo.ID
			break
		}
	}

	for i, repo := range repos {
		if errors[i] != nil {
			continue
		}
		if repo.IsFork {
			assert.Equal(t, rootID, repo.ForkID, "All forks should point to the same root")
		}
	}

	// Cleanup
	for i, repo := range repos {
		if errors[i] == nil && repo != nil {
			_ = DeleteRepositoryDirectly(t.Context(), repo.ID)
		}
	}
}
