// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forkArticle forks the article for user4 and removes the fork when the test ends.
func forkArticle(t *testing.T, repo *repo_model.Repository) {
	t.Helper()
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	fork, err := repo_service.ForkRepository(t.Context(), user4, user4, repo_service.ForkRepoOptions{
		BaseRepo:     repo,
		Name:         "user4-fork-of-repo1",
		SingleBranch: repo.DefaultBranch,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repo_service.DeleteRepositoryDirectly(t.Context(), fork.ID)
	})
}

// deleteForm builds the payload the article delete modal submits.
func deleteForm(csrf, owner, subject string) map[string]string {
	return map[string]string{
		"_csrf":               csrf,
		"action":              "delete",
		"redirect_to_article": "true",
		"article_name":        owner + "/" + subject,
	}
}

// The two notices contradict each other, so the modal must show exactly one of them.
func TestArticleSettingsDeleteModalNotices(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	session := loginUser(t, owner.Name)
	articleSettingsURL := fmt.Sprintf("/subject/%s/%s?view=article&mode=settings", subjectName, owner.Name)

	t.Run("WithoutForks", func(t *testing.T) {
		req := NewRequest(t, "GET", articleSettingsURL)
		resp := session.MakeRequest(t, req, http.StatusOK)

		notices := NewHTMLParser(t, resp.Body).Find("#article-delete-modal .warning.message").Text()
		assert.Contains(t, notices, "will permanently delete")
		assert.NotContains(t, notices, "kept as a tombstone")
	})

	t.Run("WithForks", func(t *testing.T) {
		forkArticle(t, repo)

		req := NewRequest(t, "GET", articleSettingsURL)
		resp := session.MakeRequest(t, req, http.StatusOK)

		notices := NewHTMLParser(t, resp.Body).Find("#article-delete-modal .warning.message").Text()
		assert.Contains(t, notices, "kept as a tombstone")
		assert.NotContains(t, notices, "will permanently delete")
	})
}

// Deleting a forked article leaves a tombstone, which is what the flash has to report.
func TestArticleSettingsDeleteKeepsTombstone(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	forkArticle(t, repo)

	session := loginUser(t, owner.Name)
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name),
		deleteForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
	session.MakeRequest(t, req, http.StatusSeeOther)

	tombstoned := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	require.True(t, tombstoned.IsTombstoned)
}

// A tombstone is served through its permanent repository URL only: it is the one url
// that keeps addressing the deleted article once the owner writes a new one on the same
// subject. Every other route redirects there, and the page renders the deletion notice
// instead of any content.
func TestTombstonedArticleWeb(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	forkArticle(t, repo)

	session := loginUser(t, owner.Name)
	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name),
		deleteForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
	session.MakeRequest(t, req, http.StatusSeeOther)

	repo = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	require.True(t, repo.IsTombstoned)

	repoURL := fmt.Sprintf("/%s/%s", owner.Name, repo.Name)
	require.Equal(t, repoURL, repo.Link())

	assertNotice := func(t *testing.T) {
		t.Helper()
		req := NewRequest(t, "GET", repoURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		notice := htmlDoc.Find(".history-view-section--article .warning.message").Text()
		assert.Contains(t, notice, "deleted by its author")
		// no content of the article may be rendered next to the notice
		assert.Equal(t, 0, htmlDoc.Find("#article-tabs").Length())
	}

	t.Run("PermanentURLRendersNotice", func(t *testing.T) {
		assertNotice(t)
	})

	// An archived article is deleted the same way, and its permanent URL is the only
	// route it ever had, so nothing redirects to it: the handler has to render it.
	t.Run("ArchivedTombstoneRendersNotice", func(t *testing.T) {
		require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), repo, true))
		t.Cleanup(func() {
			_ = repo_model.SetArchiveRepoState(t.Context(), repo, false)
		})
		assertNotice(t)
	})

	t.Run("OtherRoutesRedirectToPermanentURL", func(t *testing.T) {
		for _, path := range []string{
			fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name),
			repoURL + "/src/branch/" + repo.DefaultBranch,
			repoURL + "/raw/branch/" + repo.DefaultBranch + "/README.md",
			repoURL + "/commits/branch/" + repo.DefaultBranch,
		} {
			req := NewRequest(t, "GET", path)
			resp := session.MakeRequest(t, req, http.StatusSeeOther)
			assert.Equal(t, repoURL, resp.Header().Get("Location"), "%s must redirect to the notice", path)
		}
	})

	t.Run("ApiContentsIsNotFound", func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s/contents/README.md", owner.Name, repo.Name))
		session.MakeRequest(t, req, http.StatusNotFound)
	})
}

// The article owner's flow redirects to the dashboard, which renders no alert, so the
// wording is asserted on the admin listing, which does.
func TestAdminDeleteArticleTombstoneFlash(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	_, repo, _ := loadArticleRepo(t, 1)
	forkArticle(t, repo)

	session := loginUser(t, "user1")
	req := NewRequestWithValues(t, "POST", "/-/admin/repos/delete", map[string]string{
		"_csrf": GetUserCSRFToken(t, session),
		"id":    strconv.FormatInt(repo.ID, 10),
	})
	session.MakeRequest(t, req, http.StatusOK)

	tombstoned := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	require.True(t, tombstoned.IsTombstoned)

	// the flash is carried over in a cookie, so it renders on the next page
	req = NewRequest(t, "GET", "/-/admin/repos")
	resp := session.MakeRequest(t, req, http.StatusOK)

	flash := NewHTMLParser(t, resp.Body).Find(".flash-message")
	require.Equal(t, 1, flash.Length())
	assert.Contains(t, flash.Text(), "its history is kept as a tombstone, but its content is no longer readable")
}
