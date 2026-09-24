// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticlePermanentRoute covers the permanent repository URL of an article. It renders the
// same article view as the vanity URL, but always for that exact repository, so an archived
// article stays reachable once the owner has a newer active one for the same subject.
func TestArticlePermanentRoute(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	session := loginUser(t, owner.Name)
	repoURL := fmt.Sprintf("/%s/%s", owner.Name, repo.Name)

	t.Run("RendersArticleView", func(t *testing.T) {
		req := NewRequest(t, "GET", repoURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		app := htmlDoc.Find("#repo-history-app")
		require.Equal(t, 1, app.Length())
		assert.Equal(t, "article", app.AttrOr("data-initial-view", ""))
		assert.Equal(t, repo.Name, app.AttrOr("data-initial-repo", ""))
		assert.Equal(t, subjectName, app.AttrOr("data-initial-subject", ""))
		// in-page navigation must stay on the permanent URL
		assert.Equal(t, repoURL, app.AttrOr("data-article-canonical", ""))
		assert.Equal(t, 1, htmlDoc.Find(`.history-view-section--article`).Length())
	})

	// The back-link is written once above the tombstone/error/article branches, so it must
	// not reappear per branch.
	t.Run("BackLinkRenderedOnce", func(t *testing.T) {
		req := NewRequest(t, "GET", repoURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		backLink := htmlDoc.Find(`.history-view-section--article a[href*="view=bubble"]`)
		require.Equal(t, 1, backLink.Length())
		assert.Contains(t, backLink.AttrOr("href", ""), "/subject/"+subjectName)
	})

	t.Run("TabsKeepPermanentURL", func(t *testing.T) {
		req := NewRequest(t, "GET", repoURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		for _, mode := range []string{"read", "edit", "history"} {
			tab := htmlDoc.Find(fmt.Sprintf(`#article-tabs a[data-article-tab=%q]`, mode))
			require.Equal(t, 1, tab.Length(), "tab %q must be rendered", mode)
			assert.Contains(t, tab.AttrOr("href", ""), repoURL+"?")
		}
	})

	t.Run("VanityURLStillRendersArticleView", func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name))
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		app := htmlDoc.Find("#repo-history-app")
		require.Equal(t, 1, app.Length())
		assert.Equal(t, "article", app.AttrOr("data-initial-view", ""))
		assert.Equal(t, repo.Link(), app.AttrOr("data-article-canonical", ""))
	})

	// The subject route is the only entry point that does not set "ArticleLink" itself, so
	// the tabs are built from the fallback in prepareArticleView. The template deliberately
	// has none of its own: it could only hard-code the vanity url, which resolves to a
	// different article once the rendered one is archived.
	t.Run("SubjectRouteTabsFollowTheRenderedRoute", func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/subject/%s?view=article", subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		canonical := htmlDoc.Find("#repo-history-app").AttrOr("data-article-canonical", "")
		require.NotEmpty(t, canonical, "the article view must always be rendered with a link")

		for _, mode := range []string{"read", "edit", "history"} {
			tab := htmlDoc.Find(fmt.Sprintf(`#article-tabs a[data-article-tab=%q]`, mode))
			require.Equal(t, 1, tab.Length(), "tab %q must be rendered", mode)
			assert.True(t, strings.HasPrefix(tab.AttrOr("href", ""), canonical+"?"),
				"tab %q must link to %q, got %q", mode, canonical, tab.AttrOr("href", ""))
		}
	})

	t.Run("SubPathsKeepCodeView", func(t *testing.T) {
		req := NewRequest(t, "GET", repoURL+"/src/branch/"+repo.DefaultBranch)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		assert.Equal(t, 0, htmlDoc.Find("#repo-history-app").Length())
	})

	t.Run("ArchivedArticleStaysReachable", func(t *testing.T) {
		require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), repo, true))
		t.Cleanup(func() {
			_ = repo_model.SetArchiveRepoState(t.Context(), repo, false)
		})

		req := NewRequest(t, "GET", repoURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		app := htmlDoc.Find("#repo-history-app")
		require.Equal(t, 1, app.Length())
		// the owner has no other article for the subject, so the archived one is still
		// the first article of the subject hierarchy and carries no index
		assert.Equal(t, "true", app.AttrOr("data-initial-archived", ""))
		assert.Equal(t, fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name), repo.Link())
		notice := htmlDoc.Find("#article-archived-notice")
		require.Equal(t, 1, notice.Length())
		assert.False(t, notice.HasClass("tw-hidden"))
		// the read-only article has no edit tab, and the remaining tabs stay on the permanent URL
		assert.Equal(t, 0, htmlDoc.Find(`#article-tabs a[data-article-tab="edit"]`).Length())
		read := htmlDoc.Find(`#article-tabs a[data-article-tab="read"]`)
		require.Equal(t, 1, read.Length())
		assert.Contains(t, read.AttrOr("href", ""), repoURL+"?")
	})

	t.Run("ArticleURLByRepositoryNameIsNotFound", func(t *testing.T) {
		// the article namespace only resolves subject names, so a repository name that
		// is not a subject name cannot address the article
		req := NewRequest(t, "GET", fmt.Sprintf("/subject/%s/%s", repo.Name, owner.Name))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("ArticleURLByUnknownRefIsNotFound", func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/subject/%s/%s", "no-such-article", owner.Name))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	// ArticleView builds the article link straight from "subjectname" and "username"
	// without a fallback, which holds because the route cannot match an empty segment.
	t.Run("ArticleURLWithoutOwnerDoesNotReachTheArticleView", func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/subject/%s/", subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		assert.NotEqual(t, "article", htmlDoc.Find("#repo-history-app").AttrOr("data-initial-view", ""))
	})

	// The article index is 1-based and only addresses articles the owner actually holds
	// for the subject.
	t.Run("ArticleIndexOutOfRangeIsNotFound", func(t *testing.T) {
		for _, index := range []string{"0", "2"} {
			req := NewRequest(t, "GET", fmt.Sprintf("/subject/%s/%s/%s", subjectName, owner.Name, index))
			session.MakeRequest(t, req, http.StatusNotFound)
		}
	})

	// The repository name of an article is the slug of its subject, so an archived
	// article can be named exactly like a subject the owner still has an active article
	// for. Its permanent URL must keep resolving to the archived repository.
	t.Run("ArchivedArticleNamedLikeSubjectKeepsItsOwnURL", func(t *testing.T) {
		subject, err := repo_model.GetOrCreateSubject(t.Context(), repo.Name)
		require.NoError(t, err)
		other := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
		require.Equal(t, repo.OwnerID, other.OwnerID)
		require.NotEqual(t, repo.ID, other.ID)
		other.SubjectID = subject.ID
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), other, "subject_id"))

		require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), repo, true))
		t.Cleanup(func() {
			_ = repo_model.SetArchiveRepoState(t.Context(), repo, false)
		})

		// the archived article is the owner's only one for its subject, so the subject
		// hierarchy still addresses it without an index
		assert.Equal(t, fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name), repo.Link())

		req := NewRequest(t, "GET", repoURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		app := htmlDoc.Find("#repo-history-app")
		require.Equal(t, 1, app.Length())
		assert.Equal(t, repo.Name, app.AttrOr("data-initial-repo", ""))
		assert.Equal(t, subjectName, app.AttrOr("data-initial-subject", ""))
		assert.Equal(t, "true", app.AttrOr("data-initial-archived", ""))
		assert.Equal(t, repoURL, app.AttrOr("data-article-canonical", ""))
	})
}
