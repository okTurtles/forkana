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

	// User 2 creates an empty repository for this subject
	rootRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:    "first-article",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, rootRepo)

	// Verify it's initially empty and not a fork
	assert.True(t, rootRepo.IsEmpty)
	assert.False(t, rootRepo.IsFork)
	assert.Equal(t, int64(0), rootRepo.ForkID)
	assert.Positive(t, rootRepo.SubjectID)

	// Simulate User 2 adding content (e.g., committing README.md)
	// This makes the repository non-empty, which makes it eligible to be a root
	rootRepo.IsEmpty = false
	err = repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "is_empty")
	require.NoError(t, err)

	// User 4 creates an article for the same subject
	// Since User 2's repo is now non-empty, it's the root, so User 4's repo should become a fork
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

	// User 2 creates an empty repository for this subject
	rootRepo, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:    "first-article",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, rootRepo)
	assert.False(t, rootRepo.IsFork)
	assert.True(t, rootRepo.IsEmpty)

	// Simulate User 2 adding content (e.g., committing README.md)
	rootRepo.IsEmpty = false
	err = repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "is_empty")
	require.NoError(t, err)

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

func TestEmptyReposDoNotTriggerFirstArticleLogic(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Get two different users
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	subjectName := "test-empty-repos-subject"

	// User 2 creates an empty repository for this subject (no AutoInit)
	emptyRepo1, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:     "empty-article-1",
		Subject:  subjectName,
		AutoInit: false, // Empty repository
	}, true)
	require.NoError(t, err)
	require.NotNil(t, emptyRepo1)

	// Verify it's empty and not a fork
	assert.True(t, emptyRepo1.IsEmpty)
	assert.False(t, emptyRepo1.IsFork)
	assert.Equal(t, int64(0), emptyRepo1.ForkID)

	// User 4 creates another empty repository for the same subject
	// This should NOT become a fork because the first repo is empty
	emptyRepo2, err := CreateRepositoryDirectly(t.Context(), user4, user4, CreateRepoOptions{
		Name:     "empty-article-2",
		Subject:  subjectName,
		AutoInit: false, // Empty repository
	}, true)
	require.NoError(t, err)
	require.NotNil(t, emptyRepo2)

	// Verify it's also empty and NOT a fork (because first repo was empty)
	assert.True(t, emptyRepo2.IsEmpty)
	assert.False(t, emptyRepo2.IsFork, "Empty repos should not trigger first-article-becomes-root logic")
	assert.Equal(t, int64(0), emptyRepo2.ForkID)

	// Both repos should have the same subject
	assert.Equal(t, emptyRepo1.SubjectID, emptyRepo2.SubjectID)

	// Cleanup
	err = DeleteRepositoryDirectly(t.Context(), emptyRepo2.ID)
	assert.NoError(t, err)
	err = DeleteRepositoryDirectly(t.Context(), emptyRepo1.ID)
	assert.NoError(t, err)
}

func TestFirstArticleBecomesRoot_ConcurrentEmptyCreation(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Get multiple users for concurrent creation
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	subjectName := "test-concurrent-empty-subject"

	var wg sync.WaitGroup
	var mu sync.Mutex
	var repos []*repo_model.Repository
	var errs []error

	// Create empty articles concurrently from 3 users
	// Since all are empty, none should become forks
	users := []*user_model.User{user2, user4, user5}
	for _, user := range users {
		wg.Add(1)
		go func(u *user_model.User) {
			defer wg.Done()
			repo, err := CreateRepositoryDirectly(t.Context(), u, u, CreateRepoOptions{
				Name:    "concurrent-empty-article",
				Subject: subjectName,
			}, true)
			mu.Lock()
			repos = append(repos, repo)
			errs = append(errs, err)
			mu.Unlock()
		}(user)
	}

	wg.Wait()

	// Count successful creations
	var successCount, forkCount int
	for i, repo := range repos {
		if errs[i] != nil {
			continue
		}
		successCount++
		if repo.IsFork {
			forkCount++
		}
	}

	// All 3 should succeed and none should be forks (because all are empty)
	assert.Equal(t, 3, successCount, "Expected all 3 repositories to be created successfully")
	assert.Equal(t, 0, forkCount, "Expected no forks since all repositories are empty")

	// All repos should be empty
	for i, repo := range repos {
		if errs[i] != nil {
			continue
		}
		assert.True(t, repo.IsEmpty, "All repositories should be empty")
		assert.False(t, repo.IsFork, "No repository should be a fork")
	}

	// Cleanup
	for i, repo := range repos {
		if errs[i] == nil && repo != nil {
			_ = DeleteRepositoryDirectly(t.Context(), repo.ID)
		}
	}
}

func TestFirstArticleBecomesRoot_SequentialWithContent(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Get multiple users
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	subjectName := "test-sequential-content-subject"

	// User 2 creates an empty repository
	repo1, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:    "sequential-article-1",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, repo1)
	assert.True(t, repo1.IsEmpty)
	assert.False(t, repo1.IsFork)

	// User 4 creates an empty repository - should NOT be a fork (repo1 is empty)
	repo2, err := CreateRepositoryDirectly(t.Context(), user4, user4, CreateRepoOptions{
		Name:    "sequential-article-2",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, repo2)
	assert.True(t, repo2.IsEmpty)
	assert.False(t, repo2.IsFork, "Should not be a fork because repo1 is empty")

	// Simulate User 2 adding content (making repo1 the root)
	repo1.IsEmpty = false
	err = repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo1, "is_empty")
	require.NoError(t, err)

	// User 5 creates a repository - should become a fork of repo1 (which is now non-empty)
	repo3, err := CreateRepositoryDirectly(t.Context(), user5, user5, CreateRepoOptions{
		Name:    "sequential-article-3",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, repo3)
	assert.True(t, repo3.IsFork, "Should be a fork because repo1 is now non-empty")
	assert.Equal(t, repo1.ID, repo3.ForkID, "Should fork from repo1")

	// Cleanup
	_ = DeleteRepositoryDirectly(t.Context(), repo3.ID)
	_ = DeleteRepositoryDirectly(t.Context(), repo2.ID)
	_ = DeleteRepositoryDirectly(t.Context(), repo1.ID)
}

// TestFirstArticleBecomesRoot_ConcurrentWithContentRace tests the scenario where
// concurrent repository creations happen while one becomes non-empty during the race.
// This verifies that transaction isolation works correctly when determining fork status.
func TestFirstArticleBecomesRoot_ConcurrentWithContentRace(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// Get multiple users for concurrent creation
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	subjectName := "test-concurrent-race-subject"

	// User 2 creates an empty repository first
	repo1, err := CreateRepositoryDirectly(t.Context(), user2, user2, CreateRepoOptions{
		Name:    "race-article-1",
		Subject: subjectName,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, repo1)
	assert.True(t, repo1.IsEmpty)
	assert.False(t, repo1.IsFork)

	// Now simulate a race condition:
	// 1. User 2's repo becomes non-empty (simulating content being added)
	// 2. Meanwhile, User 4 and User 5 try to create repos concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex
	var repos []*repo_model.Repository
	var errs []error

	// Make repo1 non-empty to establish it as the root
	repo1.IsEmpty = false
	err = repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo1, "is_empty")
	require.NoError(t, err)

	// Now create concurrent repos - they should all become forks of repo1
	users := []*user_model.User{user4, user5}
	for _, user := range users {
		wg.Add(1)
		go func(u *user_model.User) {
			defer wg.Done()
			repo, err := CreateRepositoryDirectly(t.Context(), u, u, CreateRepoOptions{
				Name:    "race-article",
				Subject: subjectName,
			}, true)
			mu.Lock()
			repos = append(repos, repo)
			errs = append(errs, err)
			mu.Unlock()
		}(user)
	}

	wg.Wait()

	// Count results
	var successCount, forkCount int
	for i, repo := range repos {
		if errs[i] != nil {
			continue
		}
		successCount++
		if repo.IsFork {
			forkCount++
			// Verify fork points to repo1
			assert.Equal(t, repo1.ID, repo.ForkID, "Fork should point to repo1")
		}
	}

	// Both should succeed and both should be forks (because repo1 is now non-empty)
	assert.Equal(t, 2, successCount, "Expected both repositories to be created successfully")
	assert.Equal(t, 2, forkCount, "Expected both to be forks since repo1 is non-empty")

	// Cleanup
	for i, repo := range repos {
		if errs[i] == nil && repo != nil {
			_ = DeleteRepositoryDirectly(t.Context(), repo.ID)
		}
	}
	_ = DeleteRepositoryDirectly(t.Context(), repo1.ID)
}
