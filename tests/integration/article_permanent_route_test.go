// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
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
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", owner.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		app := htmlDoc.Find("#repo-history-app")
		require.Equal(t, 1, app.Length())
		assert.Equal(t, "article", app.AttrOr("data-initial-view", ""))
		assert.Equal(t, repo.Link(), app.AttrOr("data-article-canonical", ""))
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
		// the links of an archived article are built from its permanent repository URL
		assert.Equal(t, "true", app.AttrOr("data-initial-archived", ""))
		assert.Equal(t, repoURL, repo.Link())
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
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", owner.Name, repo.Name))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("ArticleURLByUnknownRefIsNotFound", func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", owner.Name, "no-such-article"))
		session.MakeRequest(t, req, http.StatusNotFound)
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

		// the archived article is addressed by its permanent URL, which cannot be
		// captured by the subject of the owner's active article
		assert.Equal(t, repoURL, repo.Link())

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
