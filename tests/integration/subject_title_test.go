// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/subjecttitle"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubjectTitleRule checks that every path creating a subject enforces the subject title
// rule (issue #401): letters, digits, spaces, hyphens and apostrophes only, starting with a
// letter or digit.
func TestSubjectTitleRule(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session := loginUser(t, user2.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)

	assertNoSubject := func(t *testing.T, name string) {
		unittest.AssertNotExistsBean(t, &repo_model.Subject{Slug: repo_model.GenerateSlugFromName(name)})
	}

	t.Run("API rejects invalid subject", func(t *testing.T) {
		for _, subject := range []string{";alskdjf", "Test: Gaudí", "Moon?", "   "} {
			req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
				Name:    "subject-title-rule-api",
				Subject: subject,
			}).AddTokenAuth(token)
			MakeRequest(t, req, http.StatusUnprocessableEntity)
		}
		assertNoSubject(t, "Moon?")
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "subject-title-rule-api"})
	})

	t.Run("API normalizes and accepts valid subject", func(t *testing.T) {
		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    "antoni-gaudis-work",
			Subject: "  Antoni   Gaudí’s  Work ",
		}).AddTokenAuth(token)
		resp := MakeRequest(t, req, http.StatusCreated)
		var apiRepo api.Repository
		DecodeJSON(t, resp, &apiRepo)
		assert.Equal(t, "Antoni Gaudí’s Work", apiRepo.Subject)

		subject := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{Slug: "antoni-gaudis-work"})
		assert.Equal(t, "Antoni Gaudí’s Work", subject.Name)
	})

	t.Run("Web create form rejects invalid subject", func(t *testing.T) {
		req := NewRequestWithValues(t, "POST", "/repo/create", map[string]string{
			"_csrf":     GetUserCSRFToken(t, session),
			"uid":       strconv.FormatInt(user2.ID, 10),
			"subject":   "-Leading Hyphen",
			"repo_name": "leading-hyphen",
		})
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		assert.Equal(t, 1, htmlDoc.doc.Find(".field.error input#subject").Length(), "the subject field should be flagged")
		assertNoSubject(t, "-Leading Hyphen")
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "leading-hyphen"})
	})

	t.Run("Web create form has the client-side pattern", func(t *testing.T) {
		resp := session.MakeRequest(t, NewRequest(t, "GET", "/repo/create"), http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		pattern, ok := htmlDoc.doc.Find("input#subject").Attr("pattern")
		require.True(t, ok)
		assert.Equal(t, subjecttitle.HTMLPattern, pattern)
	})

	t.Run("Create first article rejects invalid subject", func(t *testing.T) {
		req := NewRequest(t, "GET", "/repo/create-first-article?subject="+url.QueryEscape("#hashtag"))
		resp := session.MakeRequest(t, req, http.StatusSeeOther)
		// the subject does not exist, so its page would be a 404: go back to the subject search
		location := resp.Header().Get("Location")
		assert.Equal(t, "/explore/subjects?q="+url.QueryEscape("#hashtag"), location)
		assertNoSubject(t, "#hashtag")

		// ... which offers to create a cleaned-up, valid title instead
		resp = session.MakeRequest(t, NewRequest(t, "GET", location), http.StatusOK)
		href, ok := NewHTMLParser(t, resp.Body).Find(`a.ui.primary.button[href^="/repo/create?subject="]`).Attr("href")
		require.True(t, ok)
		assert.Equal(t, "/repo/create?subject=hashtag", href)
	})

	t.Run("Explore create offer uses a cleaned title", func(t *testing.T) {
		resp := session.MakeRequest(t, NewRequest(t, "GET", "/explore/subjects?q="+url.QueryEscape(";alskdjf")), http.StatusOK)
		href, ok := NewHTMLParser(t, resp.Body).Find(`a.ui.primary.button[href^="/repo/create?subject="]`).Attr("href")
		require.True(t, ok)
		assert.Equal(t, "/repo/create?subject=alskdjf", href)

		// nothing valid is left: no create offer
		resp = session.MakeRequest(t, NewRequest(t, "GET", "/explore/subjects?q="+url.QueryEscape("!!! ???")), http.StatusOK)
		assert.Equal(t, 0, NewHTMLParser(t, resp.Body).Find(`a[href^="/repo/create?subject="]`).Length())
	})

	t.Run("Existing subject is used whatever the spelling", func(t *testing.T) {
		existing, err := repo_model.CreateSubject(t.Context(), "Spelling Moon")
		require.NoError(t, err)

		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    "spelling-moon",
			Subject: "Spelling Moon!",
		}).AddTokenAuth(token)
		resp := MakeRequest(t, req, http.StatusCreated)
		var apiRepo api.Repository
		DecodeJSON(t, resp, &apiRepo)
		assert.Equal(t, existing.Name, apiRepo.Subject)
		assert.Equal(t, "spelling-moon", apiRepo.Name)
	})

	t.Run("Long accented title is accepted", func(t *testing.T) {
		long := strings.Repeat("é", 200) // 400 bytes, 200 characters
		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    "long-accented-subject",
			Subject: long,
		}).AddTokenAuth(token)
		resp := MakeRequest(t, req, http.StatusCreated)
		var apiRepo api.Repository
		DecodeJSON(t, resp, &apiRepo)
		assert.Equal(t, long, apiRepo.Subject)
	})

	t.Run("Existing legacy subject stays usable", func(t *testing.T) {
		legacy, err := repo_model.CreateSubject(t.Context(), "Legacy Subject")
		require.NoError(t, err)
		// simulate a subject created before the rule existed
		legacy.Name = "Legacy: Subject"
		require.NoError(t, repo_model.UpdateSubject(t.Context(), legacy))

		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    "legacy-subject",
			Subject: "Legacy: Subject",
		}).AddTokenAuth(token)
		resp := MakeRequest(t, req, http.StatusCreated)
		var apiRepo api.Repository
		DecodeJSON(t, resp, &apiRepo)
		assert.Equal(t, "Legacy: Subject", apiRepo.Subject)
	})
}
