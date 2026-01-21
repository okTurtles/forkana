// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
)

func TestGetPublicRepositoryBySubject(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Test Subject")
	assert.NoError(t, err)
	assert.NotNil(t, subject)

	// Get a repository and assign it the subject
	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	assert.NotNil(t, repo)

	repo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id")
	assert.NoError(t, err)

	// Test GetPublicRepositoryBySubject
	foundRepo, err := repo_model.GetPublicRepositoryBySubject(ctx, "Test Subject")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, repo.ID, foundRepo.ID)
	assert.NotNil(t, foundRepo.SubjectRelation)
	assert.Equal(t, subject.ID, foundRepo.SubjectRelation.ID)
	assert.Equal(t, "Test Subject", foundRepo.SubjectRelation.Name)
}

func TestGetPublicRepositoryBySubject_NotFound(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Try to get a repository with a non-existent subject
	_, err := repo_model.GetPublicRepositoryBySubject(ctx, "Non-Existent Subject")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrSubjectNotExist(err))
}

func TestGetPublicRepositoryBySubject_NoPublicRepo(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a subject without any public repository
	subject, err := repo_model.GetOrCreateSubject(ctx, "Subject Without Public Repo")
	assert.NoError(t, err)
	assert.NotNil(t, subject)

	// Try to get a public repository for this subject
	_, err = repo_model.GetPublicRepositoryBySubject(ctx, "Subject Without Public Repo")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrRepoWithSubjectNotExist(err))
}

func TestGetPublicRepositoryBySubject_PrefersRootRepo(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Shared Subject")
	assert.NoError(t, err)

	// Get two repositories - one root and one fork
	rootRepo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	rootRepo.IsFork = false
	rootRepo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, rootRepo, "subject_id", "is_fork")
	assert.NoError(t, err)

	forkRepo, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	forkRepo.IsFork = true
	forkRepo.ForkID = rootRepo.ID
	forkRepo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, forkRepo, "subject_id", "is_fork", "fork_id")
	assert.NoError(t, err)

	// GetPublicRepositoryBySubject should return the root repo, not the fork
	foundRepo, err := repo_model.GetPublicRepositoryBySubject(ctx, "Shared Subject")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, rootRepo.ID, foundRepo.ID)
	assert.False(t, foundRepo.IsFork)
}

func TestGetRepositoryByOwnerAndSubject(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Owner Subject Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo.LoadOwner(ctx)
	assert.NoError(t, err)

	repo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id")
	assert.NoError(t, err)

	// Test GetRepositoryByOwnerAndSubject
	foundRepo, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, repo.Owner.Name, "Owner Subject Test")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, repo.ID, foundRepo.ID)
	assert.NotNil(t, foundRepo.SubjectRelation)
	assert.Equal(t, subject.ID, foundRepo.SubjectRelation.ID)
}

func TestGetRepositoryByOwnerAndSubject_NotFound(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Try to get a repository with a non-existent subject
	_, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, "user1", "Non-Existent Subject")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrSubjectNotExist(err))
}

func TestGetRepositoryByOwnerAndSubject_WrongOwner(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Wrong Owner Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo.LoadOwner(ctx)
	assert.NoError(t, err)

	repo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id")
	assert.NoError(t, err)

	// Try to get the repository with a different owner
	_, err = repo_model.GetRepositoryByOwnerAndSubject(ctx, "different-user", "Wrong Owner Test")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrRepoNotExist(err))
}

func TestGetRepositoryByOwnerAndSubject_ReturnsCorrectRepo(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Multi Owner Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo1, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo1.LoadOwner(ctx)
	assert.NoError(t, err)
	repo1.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "subject_id")
	assert.NoError(t, err)

	// GetRepositoryByOwnerAndSubject should return the correct repository for the owner
	foundRepo, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, repo1.Owner.Name, "Multi Owner Test")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, repo1.ID, foundRepo.ID)
	assert.Equal(t, repo1.Owner.Name, foundRepo.OwnerName)
}

func TestGetRepositoriesBySubjectIDAndOwners(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Batch Query Test")
	assert.NoError(t, err)

	// Get two repositories and assign them the same subject
	repo1, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo1.LoadOwner(ctx)
	assert.NoError(t, err)
	repo1.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "subject_id")
	assert.NoError(t, err)

	repo2, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	err = repo2.LoadOwner(ctx)
	assert.NoError(t, err)
	repo2.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo2, "subject_id")
	assert.NoError(t, err)

	// Test fetching both repositories in a single query
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{repo1.Owner.Name, repo2.Owner.Name})
	assert.NoError(t, err)
	assert.Len(t, repos, 2)

	// Verify both repos are returned
	foundRepo1 := false
	foundRepo2 := false
	for _, r := range repos {
		if r.ID == repo1.ID {
			foundRepo1 = true
		}
		if r.ID == repo2.ID {
			foundRepo2 = true
		}
	}
	assert.True(t, foundRepo1, "repo1 should be in results")
	assert.True(t, foundRepo2, "repo2 should be in results")
}

func TestGetRepositoriesBySubjectIDAndOwners_CaseInsensitive(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Case Insensitive Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo1, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo1.LoadOwner(ctx)
	assert.NoError(t, err)
	repo1.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "subject_id")
	assert.NoError(t, err)

	// Test with different case variations
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{
		"USER2", // uppercase version of owner name
	})
	assert.NoError(t, err)
	// Should find the repo regardless of case
	assert.Len(t, repos, 1, "Should find repo regardless of case")
	assert.Equal(t, repo1.ID, repos[0].ID)
}

func TestGetRepositoriesBySubjectIDAndOwners_NoMatches(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "No Matches Test")
	assert.NoError(t, err)

	// Query with non-existent owners
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{"nonexistent1", "nonexistent2"})
	assert.NoError(t, err)
	assert.Empty(t, repos)
}

func TestGetRepositoriesBySubjectIDAndOwners_EmptyOwnerList(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Empty Owner List Test")
	assert.NoError(t, err)

	// Query with empty owner list
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{})
	assert.NoError(t, err)
	assert.Empty(t, repos)
}
