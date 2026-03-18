// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// submitChangeRequestAndGetPR creates a change request via the edit page and
// returns the PR index. Shared helper for tests in this file.
func submitChangeRequestAndGetPR(t *testing.T, session *TestSession, owner *user_model.User, repo *repo_model.Repository, content string) int64 {
	t.Helper()
	editURL := path.Join(owner.Name, repo.Name, "_edit", repo.DefaultBranch, "README.md")
	req := NewRequest(t, "GET", editURL+"?submit_change_request=true")
	resp := session.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)

	crForm := map[string]string{
		"_csrf":                 htmlDoc.GetCSRF(),
		"last_commit":           htmlDoc.GetInputValueByName("last_commit"),
		"tree_path":             "README.md",
		"content":               content,
		"commit_choice":         "direct",
		"submit_change_request": "true",
	}
	req = NewRequestWithValues(t, "POST", editURL+"?submit_change_request=true", crForm)
	resp = session.MakeRequest(t, req, http.StatusOK)

	redirectURL := test.RedirectURL(resp)
	require.Contains(t, redirectURL, "/pulls/", "Should redirect to a pull request page")

	parts := strings.Split(redirectURL, "/pulls/")
	require.Len(t, parts, 2)
	prIndex, err := strconv.ParseInt(strings.TrimSuffix(parts[1], "/"), 10, 64)
	require.NoError(t, err)
	return prIndex
}

// closePR closes a PR using the issue comments endpoint (status=close).
func closePR(t *testing.T, session *TestSession, owner *user_model.User, repo *repo_model.Repository, prIndex int64) {
	t.Helper()
	prPageURL := path.Join(owner.Name, repo.Name, "pulls", strconv.FormatInt(prIndex, 10))
	req := NewRequest(t, "GET", prPageURL)
	resp := session.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)

	closeURL := path.Join(owner.Name, repo.Name, "issues", strconv.FormatInt(prIndex, 10), "comments")
	closeForm := map[string]string{
		"_csrf":  htmlDoc.GetCSRF(),
		"status": "close",
	}
	req = NewRequestWithValues(t, "POST", closeURL, closeForm)
	session.MakeRequest(t, req, http.StatusOK)
}

// TestRejectedChangeRequestLifecycle exercises the full reject → fork-rejected-changes flow.
func TestRejectedChangeRequestLifecycle(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		sessionOwner := loginUser(t, owner.Name)
		sessionNonOwner := loginUser(t, nonOwner.Name)

		// Step 1: Create change request
		prIndex := submitChangeRequestAndGetPR(t, sessionNonOwner, owner, repo, "# Rejected lifecycle test\n\nContent that will be rejected.\n")

		pr, err := issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err)
		assert.Equal(t, repo.ID, pr.HeadRepoID, "Should be a same-repo PR")
		assert.False(t, pr.IsForked, "IsForked should be false initially")
		assert.Equal(t, int64(0), pr.ForkedRepoID, "ForkedRepoID should be 0 initially")
		headBranch := pr.HeadBranch

		// Step 2: Owner rejects (closes) the change request
		closePR(t, sessionOwner, owner, repo, prIndex)

		// Step 3: Verify PR is closed and head branch still exists
		pr, err = issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err)
		pr.Issue = unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pr.IssueID})
		assert.True(t, pr.Issue.IsClosed, "PR should be closed after rejection")
		assert.False(t, pr.HasMerged, "PR should not be merged")
		assert.True(t, gitrepo.IsBranchExist(t.Context(), repo, headBranch),
			"Head branch %q should still exist after closing", headBranch)

		// Step 4: Non-owner forks the rejected changes
		forkURL := fmt.Sprintf("/%s/%s/pulls/%d/fork_rejected_changes", owner.Name, repo.Name, prIndex)
		prPageURL := path.Join(owner.Name, repo.Name, "pulls", strconv.FormatInt(prIndex, 10))
		req := NewRequest(t, "GET", prPageURL)
		resp := sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		forkForm := map[string]string{"_csrf": htmlDoc.GetCSRF()}
		req = NewRequestWithValues(t, "POST", forkURL, forkForm)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		forkRedirect := test.RedirectURL(resp)
		require.NotEmpty(t, forkRedirect, "Should redirect after forking rejected changes")

		// Step 5: Verify is_forked and forked_repo_id are set
		pr, err = issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err)
		assert.True(t, pr.IsForked, "IsForked should be true after forking rejected changes")
		assert.Positive(t, pr.ForkedRepoID, "ForkedRepoID should be set")

		forkedRepo, err := repo_model.GetRepositoryByID(t.Context(), pr.ForkedRepoID)
		require.NoError(t, err)
		assert.Equal(t, nonOwner.ID, forkedRepo.OwnerID, "Forked repo should belong to non-owner")
		assert.True(t, forkedRepo.IsFork, "Forked repo should be a fork")

		// Step 6: Head branch cleaned up from base repo
		assert.False(t, gitrepo.IsBranchExist(t.Context(), repo, headBranch),
			"Head branch %q should be deleted from base repo after forking", headBranch)

		// Step 7: Re-forking is prevented
		req = NewRequest(t, "GET", prPageURL)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		htmlDoc = NewHTMLParser(t, resp.Body)
		forkForm["_csrf"] = htmlDoc.GetCSRF()
		req = NewRequestWithValues(t, "POST", forkURL, forkForm)
		resp = sessionNonOwner.MakeRequest(t, req, http.StatusOK)
		reforkRedirect := test.RedirectURL(resp)
		assert.NotEmpty(t, reforkRedirect, "Re-forking should redirect (with flash error)")
	})
}

// TestCloseChangeRequestWithoutFork tests the simpler reject flow where the
// author does NOT fork the changes — the is_forked field should remain false.
func TestCloseChangeRequestWithoutFork(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		nonOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		sessionNonOwner := loginUser(t, nonOwner.Name)
		sessionOwner := loginUser(t, owner.Name)

		// Step 1: Create a change request
		prIndex := submitChangeRequestAndGetPR(t, sessionNonOwner, owner, repo, "# Simple close test\n\nWill be closed without forking.\n")

		// Step 2: Owner closes the PR
		closePR(t, sessionOwner, owner, repo, prIndex)

		// Step 3: Verify PR is closed and is_forked remains false
		pr, err := issues_model.GetPullRequestByIndex(t.Context(), repo.ID, prIndex)
		require.NoError(t, err)
		pr.Issue = unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: pr.IssueID})
		assert.True(t, pr.Issue.IsClosed, "PR should be closed")
		assert.False(t, pr.HasMerged, "PR should not be merged")
		assert.False(t, pr.IsForked, "IsForked should remain false when not forked")
		assert.Equal(t, int64(0), pr.ForkedRepoID, "ForkedRepoID should remain 0")
	})
}
