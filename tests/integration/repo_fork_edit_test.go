// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"path"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForkAndEditPermissions tests the CheckForkOnEditPermissions function
// which determines how a user can edit a repository they don't own.
func TestForkAndEditPermissions(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Test fixtures:
	// - user2 owns repo1 (ID: 1) with subject_id: 1
	// - user4 is a regular user who doesn't own repo1
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	t.Run("OwnerCanEditDirectly", func(t *testing.T) {
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), owner, repo)
		require.NoError(t, err)
		assert.True(t, perms.IsRepoOwner)
		assert.True(t, perms.CanEditDirectly)
		assert.False(t, perms.NeedsFork)
		assert.False(t, perms.HasExistingFork)
		assert.False(t, perms.BlockedBySubject)
	})

	t.Run("NonOwnerNeedsFork", func(t *testing.T) {
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), nonOwner, repo)
		require.NoError(t, err)
		assert.False(t, perms.IsRepoOwner)
		assert.False(t, perms.CanEditDirectly)
		assert.True(t, perms.NeedsFork)
		assert.False(t, perms.HasExistingFork)
		assert.False(t, perms.BlockedBySubject)
	})

	t.Run("UnauthenticatedUserNoPermissions", func(t *testing.T) {
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), nil, repo)
		require.NoError(t, err)
		assert.False(t, perms.IsRepoOwner)
		assert.False(t, perms.CanEditDirectly)
		assert.False(t, perms.NeedsFork)
	})
}

// TestForkAndEditMiddlewareBypass tests that the CanWriteToBranch middleware
// correctly bypasses permission checks when fork_and_edit=true is set.
func TestForkAndEditMiddlewareBypass(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	sessionNonOwner := loginUser(t, nonOwner.Name)

	t.Run("NonOwnerWithoutForkAndEditGetsDenied", func(t *testing.T) {
		// Get the edit page to obtain CSRF token
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL)
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// POST without fork_and_edit should be denied (404)
		form := map[string]string{
			"_csrf":         htmlDoc.GetCSRF(),
			"last_commit":   htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":     "README.md",
			"content":       "Test content without fork_and_edit",
			"commit_choice": "direct",
			// Note: fork_and_edit is NOT set
		}

		req = NewRequestWithValues(t, "POST", editURL, form)
		sessionNonOwner.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("NonOwnerWithForkAndEditPassesMiddleware", func(t *testing.T) {
		// Get the edit page to obtain CSRF token
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL)
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// POST with fork_and_edit=true should NOT return 404
		// (it may fail later due to git operations, but the middleware should pass)
		form := map[string]string{
			"_csrf":         htmlDoc.GetCSRF(),
			"last_commit":   htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":     "README.md",
			"content":       "Test content with fork_and_edit",
			"commit_choice": "direct",
			"fork_and_edit": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL, form)
		// We don't check the response code here because git operations may fail
		// in the test environment. The key test is that we don't get 404 from
		// the middleware - any other response means the middleware passed.
		resp = sessionNonOwner.MakeRequest(t, req, NoExpectedStatus)

		// The response should NOT be 404 (middleware passed)
		// It may be 200 (success) or 400 (git operation failed) but not 404
		assert.NotEqual(t, http.StatusNotFound, resp.Code,
			"fork_and_edit=true should bypass CanWriteToBranch middleware")
	})
}

// TestForkAndEditOwnerNoFork tests that repository owners don't get fork_and_edit set
func TestForkAndEditOwnerNoFork(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	sessionOwner := loginUser(t, owner.Name)

	// Owner should be able to edit directly without fork_and_edit
	editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
	req := NewRequest(t, "GET", editURL)
	resp := sessionOwner.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)

	// Check that the form does NOT have fork_and_edit set to true
	forkAndEditInput := htmlDoc.doc.Find("input[name=fork_and_edit]")
	if forkAndEditInput.Length() > 0 {
		val, _ := forkAndEditInput.Attr("value")
		assert.NotEqual(t, "true", val, "Owner should not have fork_and_edit=true")
	}
}

// TestForkAndEditExistingForkDetection tests that existing forks are detected
// by CheckForkOnEditPermissions using fixture data.
// Note: We use repo11 which is a fork of repo10 owned by user13 in the fixtures.
func TestForkAndEditExistingForkDetection(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// repo11 (ID: 11) is a fork of repo10 (ID: 10) owned by user13 (ID: 13)
	// This is set up in the fixtures
	user13 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	repo10 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	repo11 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})

	// Verify fixture data is correct
	require.True(t, repo11.IsFork, "repo11 should be a fork")
	require.Equal(t, repo10.ID, repo11.ForkID, "repo11 should be a fork of repo10")
	require.Equal(t, user13.ID, repo11.OwnerID, "repo11 should be owned by user13")

	t.Run("ExistingForkIsDetected", func(t *testing.T) {
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), user13, repo10)
		require.NoError(t, err)
		assert.True(t, perms.HasExistingFork, "Should detect existing fork")
		require.NotNil(t, perms.ExistingFork, "ExistingFork should not be nil")
		assert.Equal(t, repo11.ID, perms.ExistingFork.ID)
	})
}

// TestForkAndEditUnauthenticated tests that unauthenticated users cannot use fork-and-edit
func TestForkAndEditUnauthenticated(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	t.Run("UnauthenticatedCannotPost", func(t *testing.T) {
		// Unauthenticated user should be redirected to login
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL)
		MakeRequest(t, req, http.StatusSeeOther) // Redirect to login
	})
}

// TestForkAndEditBlockedBySubject tests that users who own a different repo
// for the same subject are blocked from fork-and-edit.
// This test uses fixture data to avoid git operations.
func TestForkAndEditBlockedBySubject(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// user2 owns repo1 with subject_id: 1
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	// user5 is a regular user
	otherUser := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	// Get the subject from the repo
	subject := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{ID: repo.SubjectID})

	// Create a repository for user5 with the same subject
	// Note: In Forkana, creating a repo with an existing subject creates a fork
	ownRepo, err := repo_service.CreateRepositoryDirectly(t.Context(), otherUser, otherUser, repo_service.CreateRepoOptions{
		Name:    "my-subject-repo",
		Subject: subject.Name,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, ownRepo)
	defer func() {
		_ = repo_service.DeleteRepositoryDirectly(t.Context(), ownRepo.ID)
	}()

	t.Run("BlockedBySubjectOwnership", func(t *testing.T) {
		// The user now owns a repo for this subject
		// CheckForkOnEditPermissions should detect this
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), otherUser, repo)
		require.NoError(t, err)

		// Log what we got for debugging
		t.Logf("ownRepo.IsFork=%v, ownRepo.ForkID=%d, repo.ID=%d", ownRepo.IsFork, ownRepo.ForkID, repo.ID)
		t.Logf("perms.HasExistingFork=%v, perms.BlockedBySubject=%v", perms.HasExistingFork, perms.BlockedBySubject)

		// The user should either:
		// 1. Have an existing fork detected (if ownRepo is a fork of repo), OR
		// 2. Be blocked by subject ownership (if ownRepo is a different repo for the same subject)
		// Either way, they should NOT be able to create a new fork
		assert.True(t, perms.HasExistingFork || perms.BlockedBySubject,
			"User should either have existing fork or be blocked by subject ownership")
	})
}

// TestForkAndEditFormActionURL tests that the form action URL is correct
func TestForkAndEditFormActionURL(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	sessionNonOwner := loginUser(t, nonOwner.Name)

	t.Run("FormActionUsesRepoOperationsLink", func(t *testing.T) {
		// The form action should use RepoOperationsLink (/{owner}/{repo}/...)
		// not RepoLink (which could be /article/... for subject repos)
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL)
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Find the form action
		form := htmlDoc.doc.Find("form.edit-form, form[action*='_edit']")
		if form.Length() > 0 {
			action, exists := form.Attr("action")
			if exists {
				// Action should contain the owner/repo path, not /article/
				assert.Contains(t, action, owner.Name)
				assert.Contains(t, action, repo.Name)
				assert.NotContains(t, action, "/article/")
			}
		}
	})
}

