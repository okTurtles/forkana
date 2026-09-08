// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticleCommitLink covers the links that point at an article version, as generated
// by Repository.CommitLink and by the markup processors for commit references. The
// article route resolves a version through the "version" query parameter, it has no
// "/commit/{sha}" path.
func TestArticleCommitLink(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	session := loginUser(t, owner.Name)

	gitRepo, err := gitrepo.OpenRepository(t.Context(), repo)
	require.NoError(t, err)
	defer gitRepo.Close()

	sha, err := gitRepo.GetBranchCommitID(repo.DefaultBranch)
	require.NoError(t, err)

	articleURL := fmt.Sprintf("/article/%s/%s", url.PathEscape(owner.Name), url.PathEscape(subjectName))

	t.Run("CommitLinkRendersArticleContent", func(t *testing.T) {
		link := repo.CommitLink(sha)
		require.Equal(t, articleURL+"?version="+sha, link)

		req := NewRequest(t, "GET", link)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		app := htmlDoc.Find("#repo-history-app")
		require.Equal(t, 1, app.Length())
		assert.Equal(t, "article", app.AttrOr("data-initial-view", ""))
		require.Equal(t, 1, htmlDoc.Find(".history-view-section--article").Length())
		assert.NotEmpty(t, htmlDoc.Find(".file-view.markup").Text())
	})

	// The link built by markup.commitCrossReferencePatternProcessor for an
	// "owner/subject@sha" reference must resolve to the same route.
	t.Run("CrossReferenceLinkResolves", func(t *testing.T) {
		req := NewRequest(t, "GET", articleURL+"?version="+sha)
		session.MakeRequest(t, req, http.StatusOK)

		req = NewRequest(t, "GET", articleURL+"?version="+sha[:7])
		session.MakeRequest(t, req, http.StatusOK)
	})

	t.Run("CommitPathSuffixIsNotFound", func(t *testing.T) {
		// the article namespace has no "/commit/{sha}" route, links must use "?version="
		req := NewRequest(t, "GET", articleURL+"/commit/"+sha)
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("UnknownVersionIsNotFound", func(t *testing.T) {
		req := NewRequest(t, "GET", articleURL+"?version=0123456789abcdef0123456789abcdef01234567")
		session.MakeRequest(t, req, http.StatusNotFound)

		// a version that cannot be a commit ID is rejected before touching git
		req = NewRequest(t, "GET", articleURL+"?version=not-a-commit")
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	// A subject name is a display name, so the link generation has to escape it and the
	// route has to resolve the escaped form back to the subject.
	t.Run("SubjectWithSpecialCharacters", func(t *testing.T) {
		subject, err := repo_model.GetOrCreateSubject(t.Context(), "Fix 404 & Ünicode")
		require.NoError(t, err)

		originalSubjectID := repo.SubjectID
		repo.SubjectID = subject.ID
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "subject_id"))
		t.Cleanup(func() {
			repo.SubjectID = originalSubjectID
			_ = repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "subject_id")
		})

		reloaded := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		require.NoError(t, reloaded.LoadSubject(t.Context()))

		link := reloaded.CommitLink(sha)
		require.Equal(t, fmt.Sprintf("/article/%s/%s?version=%s", url.PathEscape(owner.Name), url.PathEscape(subject.Name), sha), link)

		req := NewRequest(t, "GET", link)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		assert.NotEmpty(t, htmlDoc.Find(".file-view.markup").Text())
	})

	// An archived article stays reachable through the article view, so its commit link
	// keeps the "version" query parameter.
	t.Run("ArchivedArticleUsesArticleVersionLink", func(t *testing.T) {
		require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), repo, true))
		t.Cleanup(func() {
			_ = repo_model.SetArchiveRepoState(t.Context(), repo, false)
		})

		reloaded := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		require.NoError(t, reloaded.LoadSubject(t.Context()))

		link := reloaded.CommitLink(sha)
		require.Equal(t, fmt.Sprintf("/article/%s/%s?version=%s", url.PathEscape(owner.Name), url.PathEscape(reloaded.GetSubject(t.Context())), sha), link)

		req := NewRequest(t, "GET", link)
		session.MakeRequest(t, req, http.StatusOK)
	})
}
