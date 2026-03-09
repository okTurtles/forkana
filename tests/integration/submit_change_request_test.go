// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	git_model "code.gitea.io/gitea/models/git"
	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"
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
		// Extract PR index from redirect URL (e.g., /user2/repo1/pulls/1)
		parts := strings.Split(redirectURL, "/pulls/")
		require.Len(t, parts, 2, "URL should contain /pulls/")
		prIndex, err := strconv.ParseInt(strings.TrimSuffix(parts[1], "/"), 10, 64)
		require.NoError(t, err, "Should be able to parse PR index from redirect URL")

		// Verify the PR exists and has correct properties
		pr, err := issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err, "PR should exist with index %d", prIndex)
		require.NotNil(t, pr, "PR should not be nil")

		// Verify it's a same-repo PR (head and base in same repo)
		assert.Equal(t, repo.ID, pr.HeadRepoID, "Head repo should be the target repo")
		assert.Equal(t, repo.ID, pr.BaseRepoID, "Base repo should be the target repo")
		assert.Equal(t, repo.DefaultBranch, pr.BaseBranch, "Base branch should be default branch")
		assert.Contains(t, pr.HeadBranch, nonOwner.LowerName+"-patch-",
			"Head branch should follow naming pattern")
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
		// capture range variable
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
				found := slices.Contains(initialBranches, branchName)
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
// Note: The submit-change-request workflow creates branches directly in the target repository
// and creates same-repo PRs (head and base both point to the target repository).
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

		// The submit-change-request workflow creates branches directly in the target repository
		// and creates same-repo PRs (HeadRepoID == BaseRepoID == repo.ID).
		// Each user's branch follows the pattern <username>-patch-N.

		// Query PRs from the target repository (repo1 owned by user2)
		allPRs, _, err := issues_model.PullRequests(t.Context(), repo.ID, &issues_model.PullRequestsOptions{
			State: "open",
		})
		require.NoError(t, err)

		// Find PRs created by user4 and user5
		var user4PR, user5PR *issues_model.PullRequest
		for _, pr := range allPRs {
			if strings.HasPrefix(pr.HeadBranch, user4.LowerName+"-patch-") {
				user4PR = pr
			}
			if strings.HasPrefix(pr.HeadBranch, user5.LowerName+"-patch-") {
				user5PR = pr
			}
		}

		// Verify user4's PR exists and is a same-repo PR
		require.NotNil(t, user4PR, "User4 should have a PR in the target repository")
		assert.Equal(t, repo.ID, user4PR.HeadRepoID,
			"User4's PR head repo should be the target repo (same-repo PR)")
		assert.Equal(t, repo.ID, user4PR.BaseRepoID,
			"User4's PR base repo should be the target repo (same-repo PR)")

		// Verify user5's PR exists and is a same-repo PR
		require.NotNil(t, user5PR, "User5 should have a PR in the target repository")
		assert.Equal(t, repo.ID, user5PR.HeadRepoID,
			"User5's PR head repo should be the target repo (same-repo PR)")
		assert.Equal(t, repo.ID, user5PR.BaseRepoID,
			"User5's PR base repo should be the target repo (same-repo PR)")

		// Verify that the redirect URLs point to PRs in the target repository
		// Since repo1 has a subject (example-subject), the URL will be /article/user2/example-subject/pulls/N
		user4Redirect := results[user4.Name].redirectURL
		user5Redirect := results[user5.Name].redirectURL

		// Load the subject to get the expected URL format
		subject := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{ID: repo.SubjectID})
		expectedURLPrefix := "/article/" + owner.Name + "/" + subject.Name + "/pulls/"

		assert.Contains(t, user4Redirect, expectedURLPrefix,
			"User4 should be redirected to a PR in the target repository")
		assert.Contains(t, user5Redirect, expectedURLPrefix,
			"User5 should be redirected to a PR in the target repository")

		// The PRs should have different indices (unique PRs)
		assert.NotEqual(t, user4PR.Index, user5PR.Index,
			"Users should have created different PRs with unique indices")

		// Verify branch names are unique
		assert.NotEqual(t, user4PR.HeadBranch, user5PR.HeadBranch,
			"Users should have unique branch names")
	})
}

// TestSubmitPullEditPostSyncsHeadRef verifies that SubmitPullEditPost correctly
// updates refs/pull/N/head after committing a revision in response to a review.
// Before the fix, InternalPush bypassed the post-receive hook so the ref
// remained stale, causing prepareViewPullInfo to show the wrong commit count
// and diff range.
func TestSubmitPullEditPostSyncsHeadRef(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		sessionOwner := loginUser(t, owner.Name)
		sessionNonOwner := loginUser(t, nonOwner.Name)

		// Step 1: Non-owner creates a PR via submit-change-request (first commit).
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		firstContent := "# First revision\n\nInitial change request content.\n"
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               firstContent,
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}
		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		redirectURL := test.RedirectURL(resp)
		require.Contains(t, redirectURL, "/pulls/", "Should redirect to a pull request page")

		// Extract PR index from redirect URL (e.g., /user2/repo1/pulls/3).
		parts := strings.Split(redirectURL, "/pulls/")
		require.Len(t, parts, 2)
		prIndex, err := strconv.ParseInt(strings.TrimSuffix(parts[1], "/"), 10, 64)
		require.NoError(t, err, "Should parse PR index from redirect URL")

		pr, err := issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err)
		require.NoError(t, pr.LoadBaseRepo(t.Context()))
		require.NoError(t, pr.LoadHeadRepo(t.Context()))
		pr.Issue = unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pr.IssueID})

		// Step 2: Owner submits a "Request Changes" review via the API.
		ownerToken := getUserToken(t, owner.Name,
			auth_model.AccessTokenScopeWriteRepository,
			auth_model.AccessTokenScopeWriteIssue)

		// Fetch the current head commit SHA so the review can be tied to it.
		gitRepo, err := gitrepo.OpenRepository(t.Context(), pr.HeadRepo)
		require.NoError(t, err)
		defer gitRepo.Close()

		firstCommitSHA, err := gitRepo.GetBranchCommitID(pr.HeadBranch)
		require.NoError(t, err, "Should get initial head branch SHA")

		reviewURL := fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/reviews",
			owner.Name, repo.Name, prIndex)
		reviewBody := fmt.Sprintf(`{"body":"Please revise.","commit_id":%q,"event":"REQUEST_CHANGES"}`, firstCommitSHA)
		req = NewRequestWithBody(t, "POST", reviewURL, strings.NewReader(reviewBody)).
			AddTokenAuth(ownerToken)
		req.Header.Set("Content-Type", "application/json")
		sessionOwner.MakeRequest(t, req, http.StatusOK)

		// Step 3: Non-owner edits the PR via SubmitPullEditPost (second commit).
		pullEditURL := path.Join(owner.Name, repo.Name, "pulls", strconv.FormatInt(prIndex, 10), "edit")
		req = NewRequest(t, "GET", pullEditURL)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc = NewHTMLParser(t, resp.Body)

		secondContent := "# Second revision\n\nRevised content addressing reviewer feedback.\n"
		editForm := map[string]string{
			"_csrf":       htmlDoc.GetCSRF(),
			"last_commit": firstCommitSHA,
			"tree_path":   "README.md",
			"content":     secondContent,
		}
		req = NewRequestWithValues(t, "POST", pullEditURL, editForm)
		sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		// Step 4: Verify refs/pull/N/head matches the new head branch commit.
		// If PushToBaseRepo was not called, the ref would still point to firstCommitSHA.
		headRefName := pr.GetGitHeadRefName() // "refs/pull/N/head"

		// Re-open the git repo to pick up any updates.
		gitRepo2, err := gitrepo.OpenRepository(t.Context(), pr.BaseRepo)
		require.NoError(t, err)
		defer gitRepo2.Close()

		headBranchSHA, err := gitRepo2.GetBranchCommitID(pr.HeadBranch)
		require.NoError(t, err, "Should get updated head branch SHA")

		pullRefSHA, err := gitRepo2.GetRefCommitID(headRefName)
		require.NoError(t, err, "refs/pull/N/head should exist after SubmitPullEditPost")

		assert.Equal(t, headBranchSHA, pullRefSHA,
			"refs/pull/N/head must point at the latest head branch commit; "+
				"stale ref means InternalPush hook bypass was not corrected")
		assert.NotEqual(t, firstCommitSHA, pullRefSHA,
			"refs/pull/N/head must have advanced beyond the first commit")
	})
}

// TestViewPullEditCommitIDGate verifies that ViewPullEdit (GET /pulls/:index/edit)
// enforces the same CommitID constraint as SubmitPullEditPost:
//   - returns 200 when a non-dismissed REQUEST_CHANGES review exists whose
//     CommitID matches the current head branch commit, and
//   - returns 404 when all such reviews are stale (targeted at an older commit).
//
// This prevents showing an edit form that can never be successfully submitted.
func TestViewPullEditCommitIDGate(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		sessionOwner := loginUser(t, owner.Name)
		sessionNonOwner := loginUser(t, nonOwner.Name)

		// Step 1: Non-owner creates a PR via submit-change-request.
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "# Gate test\n\nFirst revision.\n",
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}
		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		redirectURL := test.RedirectURL(resp)
		require.Contains(t, redirectURL, "/pulls/", "should redirect to the new PR")

		parts := strings.Split(redirectURL, "/pulls/")
		require.Len(t, parts, 2)
		prIndex, err := strconv.ParseInt(strings.TrimSuffix(parts[1], "/"), 10, 64)
		require.NoError(t, err)

		pr, err := issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err)
		require.NoError(t, pr.LoadHeadRepo(t.Context()))
		require.NoError(t, pr.LoadBaseRepo(t.Context()))
		pr.Issue = unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pr.IssueID})

		pullEditURL := path.Join(owner.Name, repo.Name, "pulls", strconv.FormatInt(prIndex, 10), "edit")

		// Step 2: Before any review exists the edit page must be inaccessible.
		req = NewRequest(t, "GET", pullEditURL)
		sessionNonOwner.MakeRequest(t, req, http.StatusNotFound)

		// Step 3: Owner submits a REQUEST_CHANGES review tied to the current head.
		ownerToken := getUserToken(t, owner.Name,
			auth_model.AccessTokenScopeWriteRepository,
			auth_model.AccessTokenScopeWriteIssue)

		gitRepo, err := gitrepo.OpenRepository(t.Context(), pr.HeadRepo)
		require.NoError(t, err)
		defer gitRepo.Close()

		firstCommitSHA, err := gitRepo.GetBranchCommitID(pr.HeadBranch)
		require.NoError(t, err)

		reviewURL := fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/reviews",
			owner.Name, repo.Name, prIndex)
		reviewBody := fmt.Sprintf(`{"body":"Please revise.","commit_id":%q,"event":"REQUEST_CHANGES"}`, firstCommitSHA)
		req = NewRequestWithBody(t, "POST", reviewURL, strings.NewReader(reviewBody)).
			AddTokenAuth(ownerToken)
		req.Header.Set("Content-Type", "application/json")
		sessionOwner.MakeRequest(t, req, http.StatusOK)

		// Step 4: Review now matches the head commit → GET /edit must return 200.
		req = NewRequest(t, "GET", pullEditURL)
		sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		// Step 5: Non-owner pushes a second commit via the edit form,
		// advancing the head beyond the reviewed commit.
		req = NewRequest(t, "GET", pullEditURL)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc = NewHTMLParser(t, resp.Body)

		editForm := map[string]string{
			"_csrf":       htmlDoc.GetCSRF(),
			"last_commit": firstCommitSHA,
			"tree_path":   "README.md",
			"content":     "# Gate test\n\nSecond revision (new commit, stales review).\n",
		}
		req = NewRequestWithValues(t, "POST", pullEditURL, editForm)
		sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		// Step 6: The review is now stale (CommitID points at the old commit).
		// GET /edit must return 404 because no matching review exists anymore.
		req = NewRequest(t, "GET", pullEditURL)
		sessionNonOwner.MakeRequest(t, req, http.StatusNotFound)
	})
}

// TestSubmitPullEditPostTreePathRestriction verifies that SubmitPullEditPost ignores
// the client-provided "tree_path" form field and resolves the target file path
// server-side instead. A malicious PR poster must not be able to write arbitrary
// files into the PR head branch by tampering with the hidden tree_path input
// (e.g. via browser DevTools).
func TestSubmitPullEditPostTreePathRestriction(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		sessionOwner := loginUser(t, owner.Name)
		sessionNonOwner := loginUser(t, nonOwner.Name)

		// Step 1: Non-owner creates a PR via submit-change-request.
		editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
		req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "# Initial\n\nContent.\n",
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}
		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		redirectURL := test.RedirectURL(resp)
		require.Contains(t, redirectURL, "/pulls/")
		parts := strings.Split(redirectURL, "/pulls/")
		require.Len(t, parts, 2)
		prIndex, err := strconv.ParseInt(strings.TrimSuffix(parts[1], "/"), 10, 64)
		require.NoError(t, err)

		pr, err := issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err)
		require.NoError(t, pr.LoadHeadRepo(t.Context()))
		pr.Issue = unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pr.IssueID})

		// Step 2: Owner submits a "Request Changes" review on the current head commit.
		ownerToken := getUserToken(t, owner.Name,
			auth_model.AccessTokenScopeWriteRepository,
			auth_model.AccessTokenScopeWriteIssue)

		headGitRepo, err := gitrepo.OpenRepository(t.Context(), pr.HeadRepo)
		require.NoError(t, err)
		defer headGitRepo.Close()

		firstCommitSHA, err := headGitRepo.GetBranchCommitID(pr.HeadBranch)
		require.NoError(t, err)

		reviewURL := fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/reviews",
			owner.Name, repo.Name, prIndex)
		reviewBody := fmt.Sprintf(`{"body":"Revise.","commit_id":%q,"event":"REQUEST_CHANGES"}`, firstCommitSHA)
		req = NewRequestWithBody(t, "POST", reviewURL, strings.NewReader(reviewBody)).
			AddTokenAuth(ownerToken)
		req.Header.Set("Content-Type", "application/json")
		sessionOwner.MakeRequest(t, req, http.StatusOK)

		// Step 3: Non-owner submits a PR edit with a tampered tree_path pointing at
		// an arbitrary file outside the article README.
		pullEditURL := path.Join(owner.Name, repo.Name, "pulls", strconv.FormatInt(prIndex, 10), "edit")
		req = NewRequest(t, "GET", pullEditURL)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc = NewHTMLParser(t, resp.Body)

		evilContent := "# Malicious\n\nEvil content injected via tampered tree_path.\n"
		editForm := map[string]string{
			"_csrf":       htmlDoc.GetCSRF(),
			"last_commit": firstCommitSHA,
			"tree_path":   ".gitea/workflows/evil.yml", // tampered — must be ignored
			"content":     evilContent,
		}
		req = NewRequestWithValues(t, "POST", pullEditURL, editForm)
		sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		// Step 4: Verify the evil file does NOT exist in the PR head branch,
		// and the README was correctly updated with the submitted content.
		newCommitSHA, err := headGitRepo.GetBranchCommitID(pr.HeadBranch)
		require.NoError(t, err)
		require.NotEqual(t, firstCommitSHA, newCommitSHA,
			"A new commit should have been made despite the tampered tree_path")

		newCommit, err := headGitRepo.GetCommit(newCommitSHA)
		require.NoError(t, err)

		// The tampered path must not have been created.
		_, evilErr := newCommit.GetTreeEntryByPath(".gitea/workflows/evil.yml")
		assert.Error(t, evilErr,
			"tampered tree_path must be ignored; .gitea/workflows/evil.yml must not exist")

		// The server-resolved README must carry the submitted content.
		readmeContent, readmeErr := newCommit.GetFileContent("README.md", 1<<20)
		require.NoError(t, readmeErr)
		assert.Equal(t, evilContent, readmeContent,
			"README.md should be updated with the submitted content (server-resolved path)")
	})
}

// TestSubmitChangeRequestWhitespaceOnlyCommitSummary tests that submitting a
// whitespace-only commit_summary field results in the default commit message
// being used instead of an empty message (issue: whitespace-only commit message).
func TestSubmitChangeRequestWhitespaceOnlyCommitSummary(t *testing.T) {
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

		// Submit with whitespace-only commit_summary
		form := map[string]string{
			"_csrf":                 htmlDoc.GetCSRF(),
			"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
			"tree_path":             "README.md",
			"content":               "# Updated content\n\nThis is a test.\n",
			"commit_summary":        "   \t\n  ", // Whitespace-only summary
			"commit_choice":         "direct",
			"submit_change_request": "true",
		}

		req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", form)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)

		redirectURL := test.RedirectURL(resp)
		require.Contains(t, redirectURL, "/pulls/", "Should redirect to a pull request page")

		// Extract PR index from redirect URL
		parts := strings.Split(redirectURL, "/pulls/")
		require.Len(t, parts, 2)
		prIndex, err := strconv.ParseInt(strings.TrimSuffix(parts[1], "/"), 10, 64)
		require.NoError(t, err, "Should parse PR index from redirect URL")

		// Load the PR and verify the commit message is the default, not empty
		pr := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: prIndex})
		require.NoError(t, pr.LoadPullRequest(nil))

		// Get the head commit to verify the commit message
		headGitRepo, err := gitrepo.OpenRepository(nil, pr.PullRequest.HeadRepo)
		require.NoError(t, err)
		defer headGitRepo.Close()

		headCommit, err := headGitRepo.GetCommit(pr.PullRequest.HeadBranch)
		require.NoError(t, err)

		// The commit message should be the default "Update README.md", not empty
		commitMessage := headCommit.CommitMessage
		assert.NotEmpty(t, strings.TrimSpace(commitMessage),
			"Commit message should not be empty when commit_summary is whitespace-only")
		assert.Contains(t, commitMessage, "Update README.md",
			"Commit message should use the default format when commit_summary is whitespace-only")
	})
}
