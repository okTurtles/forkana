// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"code.gitea.io/gitea/models/perm"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/test"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadArticleRepo returns the owner, the repository and its subject name, skipping
// the test when the fixture has no subject attached.
func loadArticleRepo(t *testing.T, repoID int64) (*user_model.User, *repo_model.Repository, string) {
	t.Helper()
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})

	require.NoError(t, repo.LoadSubject(t.Context()))
	if repo.SubjectRelation == nil {
		t.Skipf("repo %d has no subject, skipping", repoID)
	}
	return owner, repo, repo.SubjectRelation.Name
}

// archiveForm builds the payload the article archive modal submits.
func archiveForm(csrf, owner, subject string) map[string]string {
	return map[string]string{
		"_csrf":               csrf,
		"action":              "archive",
		"redirect_to_article": "true",
		"article_name":        owner + "/" + subject,
	}
}

func TestArticleSettingsArchiveSuccess(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	assert.False(t, repo.IsArchived)

	session := loginUser(t, owner.Name)
	settingsURL := fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name)

	// the archived notice is rendered but hidden while the article is not archived
	req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s?view=article", owner.Name, subjectName))
	resp := session.MakeRequest(t, req, http.StatusOK)
	notice := NewHTMLParser(t, resp.Body).Find("#article-archived-notice")
	require.Equal(t, 1, notice.Length())
	assert.True(t, notice.HasClass("tw-hidden"))
	assert.Empty(t, strings.TrimSpace(notice.Text()))

	req = NewRequestWithValues(t, "POST", settingsURL,
		archiveForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
	resp = session.MakeRequest(t, req, http.StatusSeeOther)

	articleSettingsURL := fmt.Sprintf("/article/%s/%s?view=article&mode=settings", owner.Name, subjectName)
	assert.Equal(t, articleSettingsURL, test.RedirectURL(resp))

	// the flash message is carried over in a cookie, so it renders on the next page
	req = NewRequest(t, "GET", articleSettingsURL)
	resp = session.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)

	flash := htmlDoc.Find(".flash-message")
	require.Equal(t, 1, flash.Length())
	assert.Contains(t, flash.Text(), "This article has been archived.")

	archived := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	assert.True(t, archived.IsArchived)
}

func TestArticleArchivedReadOnly(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	require.NoError(t, repo_model.SetArchiveRepoState(t.Context(), repo, true))

	session := loginUser(t, owner.Name)
	articleURL := fmt.Sprintf("/article/%s/%s?view=article", owner.Name, subjectName)

	// the notice sits above the article section, so it renders on every mode
	for _, mode := range []string{"read", "history", "settings"} {
		t.Run("Notice_"+mode, func(t *testing.T) {
			req := NewRequest(t, "GET", articleURL+"&mode="+mode)
			resp := session.MakeRequest(t, req, http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)

			notice := htmlDoc.Find("#article-archived-notice")
			require.Equal(t, 1, notice.Length())
			// the container is a flex box, so it is hidden by class, not by the hidden attribute
			assert.False(t, notice.HasClass("tw-hidden"))
			assert.Contains(t, notice.Text(), "This article has been archived by the owner on")
			assert.Contains(t, notice.Text(), "It is read-only.")
			// the notice must carry the archival date, not the last update date
			// (the date element renders its ISO fallback server-side)
			assert.Contains(t, notice.Text(), repo.ArchivedUnix.AsTime().Format(time.DateOnly))
			// it sits at the very top of the page, above the repository header
			assert.Equal(t, 1, notice.NextAllFiltered(".secondary-nav").Length())
			assert.Equal(t, 0, notice.PrevAllFiltered(".secondary-nav").Length())
		})
	}

	// the notice belongs to the article view only
	for _, view := range []string{"bubble", "table"} {
		t.Run("NoticeHidden_"+view, func(t *testing.T) {
			req := NewRequest(t, "GET", fmt.Sprintf("/subject/%s?view=%s", subjectName, view))
			resp := session.MakeRequest(t, req, http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)

			notice := htmlDoc.Find("#article-archived-notice")
			require.Equal(t, 1, notice.Length())
			assert.True(t, notice.HasClass("tw-hidden"))
		})
	}

	t.Run("EditTabHidden", func(t *testing.T) {
		// hidden for the owner too, not only for readers
		req := NewRequest(t, "GET", articleURL)
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)

		AssertHTMLElement(t, htmlDoc, `[data-article-tab="edit"]`, false)
		AssertHTMLElement(t, htmlDoc, `[data-article-tab="read"]`, true)
	})

	// archived write endpoints stay hidden behind a 404, as everywhere else in Gitea
	t.Run("EditorRoutesNotFound", func(t *testing.T) {
		csrf := GetUserCSRFToken(t, session)
		editorPath := fmt.Sprintf("/article/%s/%s", owner.Name, subjectName)

		for _, tc := range []struct {
			name   string
			method string
			path   string
		}{
			{"EditGet", "GET", editorPath + "/_edit/master/README.md"},
			{"EditPost", "POST", editorPath + "/_edit/master/README.md"},
			{"NewGet", "GET", editorPath + "/_new/master/new.md"},
			{"NewPost", "POST", editorPath + "/_new/master/new.md"},
			{"DeleteGet", "GET", editorPath + "/_delete/master/README.md"},
			{"DeletePost", "POST", editorPath + "/_delete/master/README.md"},
			{"UploadGet", "GET", editorPath + "/_upload/master"},
			{"UploadPost", "POST", editorPath + "/_upload/master"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var req *RequestWrapper
				if tc.method == "GET" {
					req = NewRequest(t, "GET", tc.path)
				} else {
					req = NewRequestWithValues(t, "POST", tc.path, map[string]string{
						"_csrf":         csrf,
						"tree_path":     "README.md",
						"content":       "archived write attempt",
						"commit_choice": "direct",
					})
				}
				session.MakeRequest(t, req, http.StatusNotFound)
			})
		}
	})
}

func TestArticleSettingsArchiveUnauthorized(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	settingsURL := fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name)

	t.Run("NonCollaborator", func(t *testing.T) {
		// blocked before the handler runs, by the "settings" group admin requirement
		session := loginUser(t, user4.Name)
		req := NewRequestWithValues(t, "POST", settingsURL,
			archiveForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
		session.MakeRequest(t, req, http.StatusNotFound)

		unarchived := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.False(t, unarchived.IsArchived)
	})

	t.Run("AdminCollaboratorIsNotOwner", func(t *testing.T) {
		// an admin collaborator passes the group requirement but is rejected by the
		// owner-only guard in the archive handler
		require.NoError(t, repo_service.AddOrUpdateCollaborator(t.Context(), repo, user4, perm.AccessModeAdmin))

		session := loginUser(t, user4.Name)
		req := NewRequestWithValues(t, "POST", settingsURL,
			archiveForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
		session.MakeRequest(t, req, http.StatusForbidden)

		unarchived := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.False(t, unarchived.IsArchived)
	})
}
