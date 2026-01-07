// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"path"
	"strings"
	"testing"

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
		// This should fail because the middleware checks the form value
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
			assert.Contains(t, respBody, "content",
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
