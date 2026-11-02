// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	git_model "code.gitea.io/gitea/models/git"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/test"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// editorFormAction returns the action of the editor form on the page at pagePath, the
// one form posting to an editor action (the page may hold others, such as a stopwatch).
func editorFormAction(t *testing.T, session *TestSession, pagePath, editorAction string) (string, *HTMLDoc) {
	t.Helper()
	resp := session.MakeRequest(t, NewRequest(t, "GET", pagePath), http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)
	var actions []string
	htmlDoc.Find("form.form-fetch-action").Each(func(_ int, form *goquery.Selection) {
		if action := form.AttrOr("action", ""); strings.Contains(action, "/"+editorAction+"/") {
			actions = append(actions, action)
		}
	})
	require.Len(t, actions, 1, "the %s page must hold one %s form", pagePath, editorAction)
	return actions[0], htmlDoc
}

// TestEditorFormActionWithoutSubject covers #274: a repository created through the API
// without a subject has no place in the subject hierarchy, so its editor forms, and the
// redirects after a commit, must stay on /{owner}/{repo} instead of a /subject/... url
// that cannot resolve it.
func TestEditorFormActionWithoutSubject(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		session := loginUser(t, "user2")
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)

		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:          "no-subject-editor",
			Readme:        "Default",
			AutoInit:      true,
			DefaultBranch: "main",
		}).AddTokenAuth(token)
		MakeRequest(t, req, http.StatusCreated)
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user2", Name: "no-subject-editor"})
		require.Zero(t, repo.SubjectID)
		const repoLink = "/user2/no-subject-editor"

		t.Run("EditFormPostsToRepoRoute", func(t *testing.T) {
			action, htmlDoc := editorFormAction(t, session, repoLink+"/_edit/main/README.md", "_edit")
			assert.Equal(t, repoLink+"/_edit/main/README.md", action)

			form := map[string]string{
				"_csrf":         htmlDoc.GetCSRF(),
				"last_commit":   htmlDoc.GetInputValueByName("last_commit"),
				"tree_path":     "README.md",
				"content":       "edited without a subject\n",
				"commit_choice": "direct",
			}
			resp := session.MakeRequest(t, NewRequestWithValues(t, "POST", action, form), http.StatusOK)
			redirect := test.RedirectURL(resp)
			assert.Equal(t, repoLink+"/src/branch/main/README.md", redirect)
			session.MakeRequest(t, NewRequest(t, "GET", redirect), http.StatusOK)

			resp = session.MakeRequest(t, NewRequest(t, "GET", repoLink+"/raw/branch/main/README.md"), http.StatusOK)
			assert.Equal(t, "edited without a subject\n", resp.Body.String())
		})

		t.Run("RedirectToArticleStaysOnRepoRoute", func(t *testing.T) {
			action, htmlDoc := editorFormAction(t, session, repoLink+"/_edit/main/README.md", "_edit")
			form := map[string]string{
				"_csrf":               htmlDoc.GetCSRF(),
				"last_commit":         htmlDoc.GetInputValueByName("last_commit"),
				"tree_path":           "README.md",
				"content":             "edited again\n",
				"commit_choice":       "direct",
				"redirect_to_article": "true",
			}
			resp := session.MakeRequest(t, NewRequestWithValues(t, "POST", action, form), http.StatusOK)
			redirect := test.RedirectURL(resp)
			assert.Equal(t, repoLink, redirect)
			session.MakeRequest(t, NewRequest(t, "GET", redirect), http.StatusOK)
		})

		t.Run("OtherEditorFormsPostToRepoRoute", func(t *testing.T) {
			branch, err := git_model.GetBranch(t.Context(), repo.ID, "main")
			require.NoError(t, err)

			for _, page := range []struct{ path, action string }{
				{repoLink + "/_new/main/", "_new"},
				{repoLink + "/_delete/main/README.md", "_delete"},
				{repoLink + "/_upload/main/", "_upload"},
				{repoLink + "/_diffpatch/main/", "_diffpatch"},
				{repoLink + "/_cherrypick/" + branch.CommitID + "/main/", "_cherrypick"},
			} {
				action, _ := editorFormAction(t, session, page.path, page.action)
				assert.True(t, strings.HasPrefix(action, repoLink+"/"+page.action+"/"), "%s form posts to %q", page.action, action)
			}
		})

		// a reader without a fork edits through "fork and edit": the form has no action, so
		// it posts back to the editor page, which commits to a new fork and redirects to it
		t.Run("ForkAndEditRedirectsToForkRepoRoute", func(t *testing.T) {
			user4 := loginUser(t, "user4")
			pagePath := repoLink + "/_edit/main/README.md"
			resp := user4.MakeRequest(t, NewRequest(t, "GET", pagePath), http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)
			editForm := htmlDoc.Find("form.edit.form-fetch-action")
			require.Equal(t, 1, editForm.Length())
			assert.Empty(t, editForm.AttrOr("action", ""))
			action := pagePath

			form := map[string]string{
				"_csrf":         htmlDoc.GetCSRF(),
				"last_commit":   htmlDoc.GetInputValueByName("last_commit"),
				"tree_path":     "README.md",
				"content":       "edited in a fork\n",
				"commit_choice": "direct",
				"fork_and_edit": "true",
			}
			resp = user4.MakeRequest(t, NewRequestWithValues(t, "POST", action, form), http.StatusOK)
			fork := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user4", ForkID: repo.ID})
			require.Zero(t, fork.SubjectID)
			redirect := test.RedirectURL(resp)
			assert.Equal(t, fork.OperationsLink()+"?mode=read", redirect)
			user4.MakeRequest(t, NewRequest(t, "GET", redirect), http.StatusOK)
		})

		// a repository with a subject keeps posting to its article url
		t.Run("ArticleKeepsSubjectRoute", func(t *testing.T) {
			article := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
			require.NotZero(t, article.SubjectID)
			action, _ := editorFormAction(t, session, "/user2/repo1/_edit/master/README.md", "_edit")
			assert.Equal(t, article.Link()+"/_edit/master/README.md", action)
			assert.True(t, strings.HasPrefix(action, "/subject/"))
		})
	})
}
