// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/test"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubmitChangeRequestAsForkOwner tests that fork owners can submit change requests
// to the original repository, and that proper validation prevents invalid submissions.
func TestSubmitChangeRequestAsForkOwner(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Test fixtures:
	// - repo10 (ID: 10) is the base repository (owned by user12)
	// - repo11 (ID: 11) is a fork of repo10 (owned by user13)
	// - user2 owns repo1 (ID: 1) with subject_id: 1
	forkOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	baseRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	forkRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})

	// Verify the fork relationship
	require.True(t, forkRepo.IsFork)
	require.Equal(t, baseRepo.ID, forkRepo.ForkID)
	require.Equal(t, forkOwner.ID, forkRepo.OwnerID)

	t.Run("ForkOwnerCanSubmitChangeRequest", func(t *testing.T) {
		// Fork owner should have CanSubmitChangeRequest=true
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), forkOwner, baseRepo)
		require.NoError(t, err)
		assert.True(t, perms.HasExistingFork)
		assert.True(t, perms.CanSubmitChangeRequest)
		assert.False(t, perms.BlockedBySubject)
	})

	t.Run("ForkOwnerCannotSubmitToOwnFork", func(t *testing.T) {
		// Fork owner should NOT be able to submit change requests to their own fork
		// (they should edit directly instead)
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), forkOwner, forkRepo)
		require.NoError(t, err)
		assert.True(t, perms.IsRepoOwner)
		assert.False(t, perms.CanSubmitChangeRequest)
	})

	t.Run("BaseRepoOwnerCannotSubmitChangeRequest", func(t *testing.T) {
		// Repository owner should NOT be able to submit change requests
		// (they should edit directly instead)
		baseRepoOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: baseRepo.OwnerID})
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), baseRepoOwner, baseRepo)
		require.NoError(t, err)
		assert.True(t, perms.IsRepoOwner)
		assert.False(t, perms.CanSubmitChangeRequest)
	})
}

// TestIndirectForkCanSubmitChangeRequest tests that a user who owns a fork-of-fork
// (indirect fork) can submit change requests to the root repository in the same fork tree.
func TestIndirectForkCanSubmitChangeRequest(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Create a unique subject for this test to avoid conflicts
	subject, err := repo_model.GetOrCreateSubject(t.Context(), "IndirectFork Integration Test Subject")
	require.NoError(t, err)

	// Get users for this test
	// Using repo10 (user12), repo11 (user13, already fork of repo10), repo12 (user14)
	userA := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 12}) // Root owner
	userB := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13}) // F1 owner
	userC := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 14}) // F2 owner (fork of fork)

	// Use existing repos and set up the fork chain
	rootRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})  // R - owned by userA (user12)
	fork1Repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11}) // F1 - owned by userB (user13)
	fork2Repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 12}) // F2 - owned by userC (user14)

	// Verify ownership matches our expectations
	require.Equal(t, userA.ID, rootRepo.OwnerID)
	require.Equal(t, userB.ID, fork1Repo.OwnerID)
	require.Equal(t, userC.ID, fork2Repo.OwnerID)

	// Save original values for cleanup
	originalRootSubjectID := rootRepo.SubjectID
	originalRootIsFork := rootRepo.IsFork
	originalRootForkID := rootRepo.ForkID
	originalFork1SubjectID := fork1Repo.SubjectID
	originalFork1IsFork := fork1Repo.IsFork
	originalFork1ForkID := fork1Repo.ForkID
	originalFork2SubjectID := fork2Repo.SubjectID
	originalFork2IsFork := fork2Repo.IsFork
	originalFork2ForkID := fork2Repo.ForkID

	// Set up the fork tree: R <- F1 <- F2
	rootRepo.SubjectID = subject.ID
	rootRepo.IsFork = false
	rootRepo.ForkID = 0
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "subject_id", "is_fork", "fork_id"))

	fork1Repo.SubjectID = subject.ID
	fork1Repo.IsFork = true
	fork1Repo.ForkID = rootRepo.ID // F1 is a fork of R
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), fork1Repo, "subject_id", "is_fork", "fork_id"))

	fork2Repo.SubjectID = subject.ID
	fork2Repo.IsFork = true
	fork2Repo.ForkID = fork1Repo.ID // F2 is a fork of F1 (indirect fork of R)
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), fork2Repo, "subject_id", "is_fork", "fork_id"))

	// Restore original values after test
	t.Cleanup(func() {
		rootRepo.SubjectID = originalRootSubjectID
		rootRepo.IsFork = originalRootIsFork
		rootRepo.ForkID = originalRootForkID
		if err := repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "subject_id", "is_fork", "fork_id"); err != nil {
			t.Logf("Warning: cleanup failed for rootRepo: %v", err)
		}

		fork1Repo.SubjectID = originalFork1SubjectID
		fork1Repo.IsFork = originalFork1IsFork
		fork1Repo.ForkID = originalFork1ForkID
		if err := repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), fork1Repo, "subject_id", "is_fork", "fork_id"); err != nil {
			t.Logf("Warning: cleanup failed for fork1Repo: %v", err)
		}

		fork2Repo.SubjectID = originalFork2SubjectID
		fork2Repo.IsFork = originalFork2IsFork
		fork2Repo.ForkID = originalFork2ForkID
		if err := repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), fork2Repo, "subject_id", "is_fork", "fork_id"); err != nil {
			t.Logf("Warning: cleanup failed for fork2Repo: %v", err)
		}
	})

	t.Run("IndirectForkOwnerHasCorrectPermissions", func(t *testing.T) {
		// userC (who owns F2, a fork of F1) tries to edit rootRepo (R)
		// userC should be allowed because F2 is in R's fork tree (indirect fork)
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), userC, rootRepo)
		require.NoError(t, err)
		assert.False(t, perms.IsRepoOwner, "User should not be the owner of the root repo")
		assert.False(t, perms.CanEditDirectly, "User should not be able to edit directly")
		assert.False(t, perms.NeedsFork, "User should not need a fork (they have one)")
		assert.True(t, perms.HasExistingFork, "User should have an existing fork (indirect)")
		assert.False(t, perms.BlockedBySubject, "User should NOT be blocked - their fork is in the same tree")
		assert.True(t, perms.CanSubmitChangeRequest, "User should be able to submit change requests")
		assert.NotNil(t, perms.ExistingFork, "ExistingFork should be set")
		assert.Equal(t, fork2Repo.ID, perms.ExistingFork.ID, "ExistingFork should be the user's indirect fork")
	})

	t.Run("IndirectForkOwnerCanSubmitToIntermediateFork", func(t *testing.T) {
		// userC can also submit change requests to F1 (the intermediate fork)
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), userC, fork1Repo)
		require.NoError(t, err)
		assert.True(t, perms.HasExistingFork, "User should have an existing fork")
		assert.True(t, perms.CanSubmitChangeRequest, "User should be able to submit change requests to intermediate fork")
		assert.False(t, perms.BlockedBySubject, "User should not be blocked")
	})

	t.Run("DirectForkOwnerCanStillSubmit", func(t *testing.T) {
		// userB (who owns F1, a direct fork of R) should still be able to submit to R
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), userB, rootRepo)
		require.NoError(t, err)
		assert.True(t, perms.HasExistingFork, "User should have an existing fork")
		assert.True(t, perms.CanSubmitChangeRequest, "User should be able to submit change requests")
		assert.False(t, perms.BlockedBySubject, "User should not be blocked")
	})
}

// TestBlockedByIndependentArticle tests that a user who owns an independent article
// for the same subject (not in the same fork tree) is blocked from submitting change requests.
func TestBlockedByIndependentArticle(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Create a unique subject for this test
	subject, err := repo_model.GetOrCreateSubject(t.Context(), "BlockedBySubject Integration Test Subject")
	require.NoError(t, err)

	// Get user2 (will be the root article owner) and user5 (will own the fork)
	userWithRoot := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	userWithFork := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	// Use repo2 as the root article (owned by user2)
	rootRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	// Use repo4 as the fork (owned by user5, we'll set it up as a fork)
	forkRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 4})

	// Verify ownership matches our expectations
	require.Equal(t, userWithRoot.ID, rootRepo.OwnerID)
	require.Equal(t, userWithFork.ID, forkRepo.OwnerID)

	// Set up the subject relationship: both repos have the same subject
	// rootRepo is the root (not a fork), forkRepo is a fork of rootRepo
	originalRootSubjectID := rootRepo.SubjectID
	originalRootIsFork := rootRepo.IsFork
	originalRootForkID := rootRepo.ForkID
	originalForkSubjectID := forkRepo.SubjectID
	originalForkIsFork := forkRepo.IsFork
	originalForkForkID := forkRepo.ForkID

	rootRepo.SubjectID = subject.ID
	rootRepo.IsFork = false
	rootRepo.ForkID = 0
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "subject_id", "is_fork", "fork_id"))

	forkRepo.SubjectID = subject.ID
	forkRepo.IsFork = true
	forkRepo.ForkID = rootRepo.ID
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), forkRepo, "subject_id", "is_fork", "fork_id"))

	// Restore original values after test
	t.Cleanup(func() {
		rootRepo.SubjectID = originalRootSubjectID
		rootRepo.IsFork = originalRootIsFork
		rootRepo.ForkID = originalRootForkID
		if err := repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), rootRepo, "subject_id", "is_fork", "fork_id"); err != nil {
			t.Logf("Warning: cleanup failed for rootRepo: %v", err)
		}

		forkRepo.SubjectID = originalForkSubjectID
		forkRepo.IsFork = originalForkIsFork
		forkRepo.ForkID = originalForkForkID
		if err := repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), forkRepo, "subject_id", "is_fork", "fork_id"); err != nil {
			t.Logf("Warning: cleanup failed for forkRepo: %v", err)
		}
	})

	t.Run("UserWithIndependentArticleIsBlocked", func(t *testing.T) {
		// userWithRoot tries to edit forkRepo
		// userWithRoot owns rootRepo (same subject), but rootRepo is NOT a fork of forkRepo
		// So userWithRoot should be blocked
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), userWithRoot, forkRepo)
		require.NoError(t, err)
		assert.False(t, perms.IsRepoOwner, "User should not be the owner of the fork repo")
		assert.False(t, perms.CanEditDirectly, "User should not be able to edit directly")
		assert.False(t, perms.NeedsFork, "User should not need a fork (they're blocked)")
		assert.False(t, perms.HasExistingFork, "User should not have an existing fork of this repo")
		assert.True(t, perms.BlockedBySubject, "User should be blocked because they own an independent article for this subject")
		assert.False(t, perms.CanSubmitChangeRequest, "User should not be able to submit change requests")
		assert.NotNil(t, perms.OwnRepoForSubject, "OwnRepoForSubject should be set")
		assert.Equal(t, rootRepo.ID, perms.OwnRepoForSubject.ID, "OwnRepoForSubject should be the user's root article")
	})

	t.Run("ForkOwnerCanStillSubmitToRoot", func(t *testing.T) {
		// userWithFork (who owns the fork) should still be able to submit to rootRepo
		perms, err := repo_service.CheckForkOnEditPermissions(t.Context(), userWithFork, rootRepo)
		require.NoError(t, err)
		assert.True(t, perms.HasExistingFork, "User should have an existing fork")
		assert.True(t, perms.CanSubmitChangeRequest, "User should be able to submit change requests")
		assert.False(t, perms.BlockedBySubject, "User should not be blocked")
	})
}

// TestForkOwnerSubmitChangeRequestEndToEnd tests the complete HTTP workflow for a fork owner
// submitting a change request to the original repository, including branch and PR creation.
func TestForkOwnerSubmitChangeRequestEndToEnd(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Test fixtures:
	// - repo10 (ID: 10) is the base repository (owned by user12)
	// - repo11 (ID: 11) is a fork of repo10 (owned by user13)
	forkOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 13})
	baseRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 10})
	baseRepoOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 12})
	forkRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 11})

	// Verify the fork relationship
	require.True(t, forkRepo.IsFork)
	require.Equal(t, baseRepo.ID, forkRepo.ForkID)
	require.Equal(t, forkOwner.ID, forkRepo.OwnerID)

	session := loginUser(t, forkOwner.Name)

	t.Run("ForkOwnerSubmitsChangeRequestSuccessfully", func(t *testing.T) {
		// GET the edit page to obtain CSRF token and last_commit
		editURL := path.Join(baseRepoOwner.Name, baseRepo.Name, "_edit", baseRepo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Submit a change request with new content
		newContent := "# Updated by fork owner\n\nThis is a test change from a fork owner.\n"
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               newContent,
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL, form)
		resp = session.MakeRequest(t, req, http.StatusOK)

		// Should redirect to the created pull request
		redirectURL := test.RedirectURL(resp)
		assert.NotEmpty(t, redirectURL, "Should redirect to pull request")
		assert.Contains(t, redirectURL, "/pulls/", "Should redirect to a pull request page")

		// Extract PR index from redirect URL (e.g., /user12/repo10/pulls/1)
		parts := strings.Split(redirectURL, "/pulls/")
		require.Len(t, parts, 2, "URL should contain /pulls/")
		prIndex, err := strconv.ParseInt(strings.TrimSuffix(parts[1], "/"), 10, 64)
		require.NoError(t, err, "Should be able to parse PR index from redirect URL")

		// Verify the PR exists and has correct properties
		pr, err := issues_model.GetPullRequestByIndex(t.Context(), baseRepo.ID, prIndex)
		require.NoError(t, err, "PR should exist with index %d", prIndex)
		require.NotNil(t, pr, "PR should not be nil")

		// Verify it's a same-repo PR (head and base in same repo)
		// This is the key difference for fork owners: they submit to the base repo, not their fork
		assert.Equal(t, baseRepo.ID, pr.HeadRepoID, "Head repo should be the base repo")
		assert.Equal(t, baseRepo.ID, pr.BaseRepoID, "Base repo should be the base repo")
		assert.Equal(t, baseRepo.DefaultBranch, pr.BaseBranch, "Base branch should be default branch")
		assert.Contains(t, pr.HeadBranch, forkOwner.LowerName+"-patch-",
			"Head branch should follow naming pattern with fork owner's username")

		// Note: We don't verify the branch exists because it may be cleaned up after PR creation
		// The important thing is that the PR was created successfully with the correct metadata
	})
}
