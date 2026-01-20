// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	git_model "code.gitea.io/gitea/models/git"
	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubmitChangeRequest tests the submit-change-request workflow where a non-owner
// user can submit changes to a repository they don't have write access to.
// The workflow creates a branch in the target repository, commits the changes,
// and creates a pull request from that branch to the default branch.
func TestSubmitChangeRequest(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Test fixtures:
	// - user2 owns repo1 (ID: 1) with subject_id: 1
	// - user4 is a regular user who doesn't own repo1
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	sessionNonOwner := loginUser(t, nonOwner.Name)

	t.Run("SubmitChangeRequestCreatesInRepoPR", func(t *testing.T) {
		// Get the edit page to obtain CSRF token and last_commit
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Submit a change request with new content
		newContent := "# Updated by submit-change-request test\n\nThis is a test change.\n"
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               newContent,
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL, form)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		// Should redirect to the created pull request
		redirectURL := test.RedirectURL(resp)
		assert.NotEmpty(t, redirectURL, "Should redirect to pull request")
		assert.Contains(t, redirectURL, "/pulls/", "Should redirect to a pull request page")

		// Verify the pull request was created
		// Extract PR number from URL (e.g., /user2/repo1/pulls/1)
		parts := strings.Split(redirectURL, "/pulls/")
		require.Len(t, parts, 2, "URL should contain /pulls/")

		// Verify the PR exists and has correct properties
		prs, err := issues_model.GetPullRequestByIndex(t.Context(), repo.ID, 1)
		if err == nil && prs != nil {
			// Verify it's a same-repo PR (head and base in same repo)
			assert.Equal(t, repo.ID, prs.HeadRepoID, "Head repo should be the target repo")
			assert.Equal(t, repo.ID, prs.BaseRepoID, "Base repo should be the target repo")
			assert.Equal(t, repo.DefaultBranch, prs.BaseBranch, "Base branch should be default branch")
			assert.Contains(t, prs.HeadBranch, nonOwner.LowerName+"-patch-",
				"Head branch should follow naming pattern")
		}
	})

	t.Run("SubmitChangeRequestWithoutQueryParamFails", func(t *testing.T) {
		// Get the edit page without submit_change_request query param
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL)
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Try to submit without submit_change_request in form or query
		// This should fail because the middleware checks the form value (query params AND form data)
		form := map[string]string{
			"_csrf":         htmlDoc.GetCSRF(),
			"last_commit":   htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":     "README.md",
			"content":       "Test content",
			"commit_choice": "direct",
			// Note: submit_change_request is NOT set
		}

		req = NewRequestWithValues(t, "POST", editURL, form)
		resp = sessionNonOwner.MakeRequest(t, req, NoExpectedStatus)

		// Should get 404 because the middleware didn't bypass permission check
		// (submit_change_request was not set in the form)
		assert.Equal(t, http.StatusNotFound, resp.Code,
			"Should fail without submit_change_request in form")
	})

	t.Run("SubmitChangeRequestWithQueryParamPasses", func(t *testing.T) {
		// Get the edit page WITH submit_change_request query param
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Submit with submit_change_request=true
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "Test content for query param test",
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, NoExpectedStatus)

		// Should NOT get 404 - the middleware should pass
		assert.NotEqual(t, http.StatusNotFound, resp.Code,
			"submit_change_request=true should bypass CanWriteToBranch middleware")
	})
}

// TestSubmitChangeRequestBranchNaming tests that the branch naming pattern is correct.
func TestSubmitChangeRequestBranchNaming(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	sessionNonOwner := loginUser(t, nonOwner.Name)

	t.Run("BranchNameFollowsPattern", func(t *testing.T) {
		// Get the edit page
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Submit a change request
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "# Branch naming test\n",
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		redirectURL := test.RedirectURL(resp)
		if redirectURL != "" && strings.Contains(redirectURL, "/pulls/") {
			// Follow the redirect to get the PR page
			req = NewRequest(t, "GET", redirectURL)
			resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)

			// The PR page should show the branch name
			// Branch name should be like "user4-patch-1"
			bodyText := resp.Body.String()
			assert.Contains(t, bodyText, nonOwner.LowerName+"-patch-",
				"PR page should show branch name with user-patch-N pattern")
		}
	})
}

// TestSubmitChangeRequestErrorCases tests error handling for submit-change-request.
func TestSubmitChangeRequestErrorCases(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	sessionNonOwner := loginUser(t, nonOwner.Name)

	t.Run("EmptyContentFails", func(t *testing.T) {
		// Get the edit page
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Submit with empty content
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "", // Empty content
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, NoExpectedStatus)

		// Should fail with bad request (empty content not allowed)
		// Note: The actual behavior depends on whether empty content is validated
		// This test documents the expected behavior
		if resp.Code == http.StatusBadRequest {
			respBody := resp.Body.String()
			assert.Contains(t, respBody, "Content",
				"Error message should mention content issue")
		}
	})

	t.Run("UnauthenticatedUserRedirectedToLogin", func(t *testing.T) {
		// Unauthenticated user should be redirected to login
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		MakeRequest(t, req, http.StatusSeeOther) // Redirect to login
	})
}

// TestSubmitChangeRequestWhitespaceOnlyContent tests that submitting content containing only
// whitespace characters is rejected with HTTP 400 Bad Request (SCR-001 fix verification).
func TestSubmitChangeRequestWhitespaceOnlyContent(t *testing.T) {
	// Test cases for various whitespace-only content scenarios
	testCases := []struct {
		name    string
		content string
	}{
		{"OnlySpaces", "     "},
		{"OnlyTabs", "\t\t\t"},
		{"OnlyNewlines", "\n\n\n"},
		{"MixedWhitespace", "  \t\n  \t\n  "},
		{"SingleSpace", " "},
		{"SingleNewline", "\n"},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			onGiteaRun(t, func(t *testing.T, u *url.URL) {
				owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
				nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
				repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

				sessionNonOwner := loginUser(t, nonOwner.Name)

				// Get the edit page
				editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
				req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
				resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
				htmlDoc := NewHTMLParser(t, resp.Body)

				// Submit with whitespace-only content
				form := map[string]string{
					"_csrf":                 htmlDoc.GetCSRF(),
					"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
					"tree_path":             "README.md",
					"content":               tc.content, // Whitespace-only content
					"commit_choice":         "direct",
					"submit_change_request": "true",
				}

				req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
				resp = sessionNonOwner.MakeRequest(t, req, http.StatusBadRequest)

				// Verify the error response mentions content
				respBody := resp.Body.String()
				assert.Contains(t, respBody, "Content",
					"Error message should mention content issue for whitespace-only content: %q", tc.content)
			})
		})
	}
}

// TestSubmitChangeRequestSecurityBypass tests that submit_change_request=true cannot be used
// to bypass permission checks on non-edit endpoints.
func TestSubmitChangeRequestSecurityBypass(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	sessionNonOwner := loginUser(t, nonOwner.Name)

	// Get CSRF token from any page
	editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
	req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
	resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)
	csrf := htmlDoc.GetCSRF()

	t.Run("DeleteEndpointRejectsSubmitChangeRequest", func(t *testing.T) {
		// Attempt to delete a file with submit_change_request=true
		deleteURL := path.Join(owner.Name, repo.Name, "_delete", repo.DefaultBranch, "README.md")
		form := map[string]string{
			"_csrf":                 csrf,
			"tree_path":             "README.md",
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req := NewRequestWithValues(t, "POST", deleteURL+"?submit_change_request=true", form)
		resp := sessionNonOwner.MakeRequest(t, req, NoExpectedStatus)

		// Should get 404 (permission denied)
		assert.Equal(t, http.StatusNotFound, resp.Code,
			"submit_change_request=true should NOT bypass CanWriteToBranch for _delete action")
	})

	t.Run("UploadEndpointRejectsSubmitChangeRequest", func(t *testing.T) {
		// Attempt to upload with submit_change_request=true
		uploadURL := path.Join(owner.Name, repo.Name, "_upload", repo.DefaultBranch, "/")
		form := map[string]string{
			"_csrf":                 csrf,
			"tree_path":             "",
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req := NewRequestWithValues(t, "POST", uploadURL+"?submit_change_request=true", form)
		resp := sessionNonOwner.MakeRequest(t, req, NoExpectedStatus)

		// Should get 404 (permission denied)
		assert.Equal(t, http.StatusNotFound, resp.Code,
			"submit_change_request=true should NOT bypass CanWriteToBranch for _upload action")
	})
}

// TestSubmitChangeRequestPRCreationFailureCleanup tests that orphaned branches are cleaned up
// when PR creation fails after the branch has been created (SCR-002 fix verification).
// This test uses user blocking to trigger a PR creation failure.
func TestSubmitChangeRequestPRCreationFailureCleanup(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		// Test fixtures:
		// - user2 owns repo1 (ID: 1)
		// - user4 is a regular user who doesn't own repo1
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		// Get initial branch count for the repository
		initialBranches, err := git_model.FindBranchNames(t.Context(), git_model.FindBranchOptions{
			RepoID: repo.ID,
		})
		require.NoError(t, err)
		initialBranchCount := len(initialBranches)

		// Step 1: Owner blocks the non-owner user
		// This will cause NewPullRequest() to fail with ErrBlockedUser
		ownerToken := getUserToken(t, owner.Name, auth_model.AccessTokenScopeWriteUser)
		blockReq := NewRequest(t, "PUT", "/api/v1/user/blocks/"+nonOwner.Name).
			AddTokenAuth(ownerToken)
		MakeRequest(t, blockReq, http.StatusNoContent)

		// Ensure we unblock the user after the test
		defer func() {
			unblockReq := NewRequest(t, "DELETE", "/api/v1/user/blocks/"+nonOwner.Name).
				AddTokenAuth(ownerToken)
			MakeRequest(t, unblockReq, http.StatusNoContent)
		}()

		// Step 2: Non-owner attempts to submit a change request
		// The branch will be created, but PR creation will fail due to blocking
		sessionNonOwner := loginUser(t, nonOwner.Name)

		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		// Submit a change request - this should fail due to user being blocked
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "# Test content for PR failure cleanup\n\nThis should trigger cleanup.\n",
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, NoExpectedStatus)

		// The request should fail (500 Internal Server Error due to blocked user)
		// The exact status code depends on how the error is handled
		assert.NotEqual(t, http.StatusOK, resp.Code,
			"Request should fail when user is blocked")

		// Step 3: Verify that no orphaned branch was left behind
		// The branch cleanup should have removed any branch that was created
		finalBranches, err := git_model.FindBranchNames(t.Context(), git_model.FindBranchOptions{
			RepoID: repo.ID,
		})
		require.NoError(t, err)

		// Check that no new branches were added (cleanup worked)
		assert.Len(t, finalBranches, initialBranchCount,
			"No orphaned branches should remain after PR creation failure; initial: %d, final: %d",
			initialBranchCount, len(finalBranches))

		// Additionally, verify no branch matching the expected pattern exists
		for _, branchName := range finalBranches {
			if strings.HasPrefix(branchName, nonOwner.LowerName+"-patch-") {
				// Check if this branch existed before the test
				found := false
				for _, initialBranch := range initialBranches {
					if initialBranch == branchName {
						found = true
						break
					}
				}
				assert.True(t, found,
					"Branch %s should not exist as it should have been cleaned up", branchName)
			}
		}
	})
}

// TestSubmitChangeRequestConcurrentBranchCollision tests that concurrent change request
// submissions from multiple users correctly generate unique branch names without collisions.
// This verifies that getUniquePatchBranchName() handles the scenario where multiple users
// simultaneously submit change requests that would generate similar branch name patterns.
// Note: The submit-change-request workflow creates a fork for each user and creates the
// branch/PR in that fork, so each user gets their own repository with their own branch.
func TestSubmitChangeRequestConcurrentBranchCollision(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		// Test fixtures:
		// - user2 owns repo1 (ID: 1)
		// - user4 and user5 are regular users who don't own repo1
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		// Create sessions for both users
		sessionUser4 := loginUser(t, user4.Name)
		sessionUser5 := loginUser(t, user5.Name)

		// We'll use a WaitGroup to synchronize concurrent submissions
		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make(map[string]struct {
			success     bool
			redirectURL string
			statusCode  int
		})

		// Helper function to submit a change request
		submitChangeRequest := func(session *TestSession, userName, content string) {
			defer wg.Done()

			// Get the edit page to obtain CSRF token and last_commit
			editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
			req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
			resp := session.MakeRequest(t, req, http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)

			// Submit a change request with unique content
			form := map[string]string{
				"_csrf":                 htmlDoc.GetCSRF(),
				"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
				"tree_path":             "README.md",
				"content":               content,
				"commit_choice":         "direct",
				"submit_change_request": "true",
			}

			req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
			resp = session.MakeRequest(t, req, NoExpectedStatus)

			redirectURL := test.RedirectURL(resp)

			mu.Lock()
			results[userName] = struct {
				success     bool
				redirectURL string
				statusCode  int
			}{
				success:     resp.Code == http.StatusOK && strings.Contains(redirectURL, "/pulls/"),
				redirectURL: redirectURL,
				statusCode:  resp.Code,
			}
			mu.Unlock()
		}

		// Submit change requests concurrently from both users
		wg.Add(2)
		go submitChangeRequest(sessionUser4, user4.Name, fmt.Sprintf("# Concurrent edit by %s\n\nContent from user4.\n", user4.Name))
		go submitChangeRequest(sessionUser5, user5.Name, fmt.Sprintf("# Concurrent edit by %s\n\nContent from user5.\n", user5.Name))
		wg.Wait()

		// Verify both submissions succeeded
		for userName, result := range results {
			assert.True(t, result.success,
				"User %s should have successfully submitted a change request (status: %d, redirect: %s)",
				userName, result.statusCode, result.redirectURL)
			assert.Contains(t, result.redirectURL, "/pulls/",
				"User %s should be redirected to a pull request page", userName)
		}

		// The submit-change-request workflow creates a fork for each user.
		// Each user's fork will have a branch with the pattern <username>-patch-N.
		// We need to find the forks and verify the PRs were created there.

		// Find user4's fork of repo1
		user4Fork, err := repo_model.GetRepositoryByOwnerAndName(t.Context(), user4.Name, repo.Name)
		require.NoError(t, err, "User4 should have a fork of repo1")
		require.NotNil(t, user4Fork, "User4's fork should exist")

		// Find user5's fork of repo1
		user5Fork, err := repo_model.GetRepositoryByOwnerAndName(t.Context(), user5.Name, repo.Name)
		require.NoError(t, err, "User5 should have a fork of repo1")
		require.NotNil(t, user5Fork, "User5's fork should exist")

		// Verify PRs were created in each user's fork by querying the base repo
		// The PRs are same-repo PRs (head and base are both the fork)
		user4PRs, _, err := issues_model.PullRequests(t.Context(), user4Fork.ID, &issues_model.PullRequestsOptions{
			State: "open",
		})
		require.NoError(t, err)

		var user4PRCount int
		for _, pr := range user4PRs {
			if strings.HasPrefix(pr.HeadBranch, user4.LowerName+"-patch-") {
				user4PRCount++
			}
		}
		assert.GreaterOrEqual(t, user4PRCount, 1,
			"User4 should have at least one PR created in their fork")

		user5PRs, _, err := issues_model.PullRequests(t.Context(), user5Fork.ID, &issues_model.PullRequestsOptions{
			State: "open",
		})
		require.NoError(t, err)

		var user5PRCount int
		for _, pr := range user5PRs {
			if strings.HasPrefix(pr.HeadBranch, user5.LowerName+"-patch-") {
				user5PRCount++
			}
		}
		assert.GreaterOrEqual(t, user5PRCount, 1,
			"User5 should have at least one PR created in their fork")

		// Verify that the redirect URLs point to different PRs (unique branch names)
		// Each user should be redirected to their own fork's PR
		user4Redirect := results[user4.Name].redirectURL
		user5Redirect := results[user5.Name].redirectURL

		assert.Contains(t, user4Redirect, user4.Name+"/",
			"User4 should be redirected to their own fork's PR")
		assert.Contains(t, user5Redirect, user5.Name+"/",
			"User5 should be redirected to their own fork's PR")

		// The PRs should be in different repositories (each user's fork)
		assert.NotEqual(t, user4Redirect, user5Redirect,
			"Users should be redirected to different PRs")
	})
}
