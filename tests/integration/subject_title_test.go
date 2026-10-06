// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubjectTitleRule checks that every path creating a subject enforces the subject title
// rule (issue #401), which follows Wikipedia's technical restrictions on page titles: no
// # < > [ ] | { } or control characters, no percent-encoding, HTML character references or
// "~~~", no relative path, no leading colon, at most 255 bytes; new titles are normalized and
// their first letter is capitalized.
func TestSubjectTitleRule(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session := loginUser(t, user2.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)

	assertNoSubject := func(t *testing.T, name string) {
		unittest.AssertNotExistsBean(t, &repo_model.Subject{Slug: repo_model.GenerateSlugFromName(name)})
	}
	createAPIRepo := func(t *testing.T, name, subject string, status int) *api.Repository {
		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    name,
			Subject: subject,
		}).AddTokenAuth(token)
		resp := MakeRequest(t, req, status)
		if status != http.StatusCreated {
			return nil
		}
		var apiRepo api.Repository
		DecodeJSON(t, resp, &apiRepo)
		return &apiRepo
	}

	t.Run("API rejects invalid subject", func(t *testing.T) {
		for _, subject := range []string{"a#b", "Brand [New]", "%41 Moon", "AT&amp;T Moon", "Moon~~~ x", ":Moon x", "../Moon", "Moon/..", "   "} {
			createAPIRepo(t, "subject-title-rule-api", subject, http.StatusUnprocessableEntity)
		}
		assertNoSubject(t, "a#b")
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "subject-title-rule-api"})
	})

	t.Run("API accepts punctuation", func(t *testing.T) {
		for i, subject := range []string{";alskdjf", "Test: Gaudí", "C++ (language)", "Rock & Roll!", "Who? What?"} {
			apiRepo := createAPIRepo(t, fmt.Sprintf("punctuation-%d", i), subject, http.StatusCreated)
			assert.Equal(t, subject, apiRepo.Subject)
			stored := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{Slug: repo_model.GenerateSlugFromName(subject)})
			assert.Equal(t, subject, stored.Name)
		}
	})

	t.Run("API normalizes a new subject and capitalizes its first letter", func(t *testing.T) {
		apiRepo := createAPIRepo(t, "iphone-article", "  iPhone_models\u00a0 list ", http.StatusCreated)
		assert.Equal(t, "IPhone models list", apiRepo.Subject)
		subject := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{Slug: "iphone-models-list"})
		assert.Equal(t, "IPhone models list", subject.Name)

		// the subject page lives at the capitalized title
		session.MakeRequest(t, NewRequest(t, "GET", "/subject/"+url.PathEscape("IPhone models list")), http.StatusOK)
	})

	t.Run("Web create form rejects invalid subject", func(t *testing.T) {
		req := NewRequestWithValues(t, "POST", "/repo/create", map[string]string{
			"_csrf":     GetUserCSRFToken(t, session),
			"uid":       strconv.FormatInt(user2.ID, 10),
			"subject":   ":Leading Colon",
			"repo_name": "leading-colon",
		})
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		assert.Equal(t, 1, htmlDoc.doc.Find(".field.error input#subject").Length(), "the subject field should be flagged")
		assert.Contains(t, htmlDoc.doc.Find(".ui.negative.message").Text(), "cannot start with a colon")
		assertNoSubject(t, "Leading Colon")
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "leading-colon"})
	})

	t.Run("Web create form explains a too long title in bytes", func(t *testing.T) {
		req := NewRequestWithValues(t, "POST", "/repo/create", map[string]string{
			"_csrf":     GetUserCSRFToken(t, session),
			"uid":       strconv.FormatInt(user2.ID, 10),
			"subject":   strings.Repeat("é", 200),
			"repo_name": "too-long-subject",
		})
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, NewHTMLParser(t, resp.Body).doc.Find(".ui.negative.message").Text(), "at most 255 bytes")
	})

	t.Run("Subject inputs get a non-blocking hint, not a blocking pattern", func(t *testing.T) {
		for _, link := range []string{
			"/repo/create",
			fmt.Sprintf("/repo/migrate?service_type=%d", api.PlainGitService),
			fmt.Sprintf("/repo/migrate?service_type=%d", api.GithubService),
			fmt.Sprintf("/repo/migrate?service_type=%d", api.GitlabService),
		} {
			resp := session.MakeRequest(t, NewRequest(t, "GET", link), http.StatusOK)
			htmlDoc := NewHTMLParser(t, resp.Body)
			input := htmlDoc.doc.Find("input#subject")
			_, hasPattern := input.Attr("pattern")
			assert.False(t, hasPattern, "%s: existing subjects may break the rule, the browser must not block them", link)
			// maxlength counts UTF-16 code units, which never exceed the UTF-8 bytes, so it never
			// rejects a title of at most 255 bytes; the server checks the bytes
			assert.Equal(t, "255", input.AttrOr("maxlength", ""), link)
			hint := input.Closest(".field").Find("[data-subject-title-hint]")
			require.Equal(t, 1, hint.Length(), "%s: missing subject title hint", link)
			assert.Contains(t, hint.AttrOr("data-msg-forbidden-char", ""), "# < > [ ] | { }", link)
			assert.Contains(t, hint.AttrOr("data-msg-html-entity", ""), "&amp;", link)
			assert.Contains(t, hint.AttrOr("data-msg-too-long", ""), "255 bytes", link)
			assert.Contains(t, hint.AttrOr("data-msg-normalized", ""), "%s", link)
		}
	})

	t.Run("Create form prefills the normalized title", func(t *testing.T) {
		resp := session.MakeRequest(t, NewRequest(t, "GET", "/repo/create?subject="+url.QueryEscape("eBay_history")), http.StatusOK)
		assert.Equal(t, "EBay history", NewHTMLParser(t, resp.Body).doc.Find("input#subject").AttrOr("value", ""))
	})

	t.Run("Web migrate form rejects invalid subject", func(t *testing.T) {
		req := NewRequestWithValues(t, "POST", "/repo/migrate", map[string]string{
			"_csrf":      GetUserCSRFToken(t, session),
			"uid":        strconv.FormatInt(user2.ID, 10),
			"clone_addr": "https://github.com/go-gitea/test_repo.git",
			"service":    strconv.Itoa(int(api.PlainGitService)),
			"repo_name":  "subject-title-rule-web-migrate",
			"subject":    "a#b",
		})
		resp := session.MakeRequest(t, req, http.StatusOK)
		htmlDoc := NewHTMLParser(t, resp.Body)
		assert.Equal(t, 1, htmlDoc.doc.Find(".field.error input#subject").Length(), "the subject field should be flagged")
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "subject-title-rule-web-migrate"})
	})

	t.Run("API migrate rejects invalid subject", func(t *testing.T) {
		req := NewRequestWithJSON(t, "POST", "/api/v1/repos/migrate", &api.MigrateRepoOptions{
			CloneAddr:   "https://github.com/go-gitea/test_repo.git",
			RepoOwnerID: user2.ID,
			RepoName:    "subject-title-rule-api-migrate",
			Subject:     "a#b",
		}).AddTokenAuth(token)
		MakeRequest(t, req, http.StatusUnprocessableEntity)
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "subject-title-rule-api-migrate"})
	})

	t.Run("API generate rejects invalid subject", func(t *testing.T) {
		templateRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 44, IsTemplate: true})
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/generate", templateRepo.OwnerName, templateRepo.Name), &api.GenerateRepoOption{
			Owner:      user2.Name,
			Name:       "subject-title-rule-generate",
			Subject:    "~~~",
			GitContent: true,
		}).AddTokenAuth(token)
		MakeRequest(t, req, http.StatusUnprocessableEntity)
		unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerID: user2.ID, LowerName: "subject-title-rule-generate"})
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
		assert.Equal(t, "/repo/create?subject=Hashtag", href)
	})

	t.Run("Explore create offer uses a cleaned title", func(t *testing.T) {
		offer := func(t *testing.T, keyword string) (string, bool) {
			resp := session.MakeRequest(t, NewRequest(t, "GET", "/explore/subjects?q="+url.QueryEscape(keyword)), http.StatusOK)
			return NewHTMLParser(t, resp.Body).Find(`a.ui.primary.button[href^="/repo/create?subject="]`).Attr("href")
		}
		// valid titles are offered as typed, apart from the first letter
		href, ok := offer(t, ";qwerty")
		require.True(t, ok)
		assert.Equal(t, "/repo/create?subject="+url.QueryEscape(";qwerty"), href)
		href, ok = offer(t, "xkcd comics")
		require.True(t, ok)
		assert.Equal(t, "/repo/create?subject="+url.QueryEscape("Xkcd comics"), href)

		// nothing valid is left: no create offer
		_, ok = offer(t, "#<>[]")
		assert.False(t, ok)
	})

	t.Run("Existing subject is used whatever the spelling", func(t *testing.T) {
		existing, err := repo_model.CreateSubject(t.Context(), "Spelling Moon")
		require.NoError(t, err)

		apiRepo := createAPIRepo(t, "spelling-moon", "spelling moon~~~", http.StatusCreated)
		assert.Equal(t, existing.Name, apiRepo.Subject)
		assert.Equal(t, "spelling-moon", apiRepo.Name)
	})

	t.Run("Length counts bytes", func(t *testing.T) {
		createAPIRepo(t, "long-accented-subject", strings.Repeat("é", 200), http.StatusUnprocessableEntity) // 400 bytes
		long := strings.Repeat("É", 127)                                                                    // 254 bytes
		apiRepo := createAPIRepo(t, "long-accented-subject", long, http.StatusCreated)
		assert.Equal(t, long, apiRepo.Subject)
	})

	t.Run("Existing legacy subject stays usable", func(t *testing.T) {
		for i, name := range []string{"Legacy [Bracket] Title", "legacy lowercase title", "Legacy " + strings.Repeat("ë", 200)} {
			legacy := &repo_model.Subject{Name: name, Slug: repo_model.GenerateSlugFromName(name)}
			require.NoError(t, db.Insert(t.Context(), legacy))
			apiRepo := createAPIRepo(t, fmt.Sprintf("legacy-subject-%d", i), name, http.StatusCreated)
			assert.Equal(t, name, apiRepo.Subject, "an existing subject keeps its name")
		}
	})
}

// TestSubjectTitleURLs covers titles with characters that matter in URLs. The subject is a
// path segment ("/subject/{subject}/{owner}"), so every link must escape it as one segment:
// "/" as %2F (or "/subject/AC/DC/user2" would read as the subject "AC" of the owner "DC"),
// "?" as %3F, "%" as %25; "&" and "+" are literal in a path.
func TestSubjectTitleURLs(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	session2 := loginUser(t, user2.Name)
	session4 := loginUser(t, user4.Name)
	token2 := getTokenForLoggedInUser(t, session2, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)
	token4 := getTokenForLoggedInUser(t, session4, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)

	createArticle := func(t *testing.T, token, subject string) *api.Repository {
		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:     repo_model.GenerateRepoNameFromSubject(subject),
			Subject:  subject,
			AutoInit: true,
			Readme:   "Default",
		}).AddTokenAuth(token)
		resp := MakeRequest(t, req, http.StatusCreated)
		var apiRepo api.Repository
		DecodeJSON(t, resp, &apiRepo)
		require.Equal(t, subject, apiRepo.Subject)
		return &apiRepo
	}

	// visitSubjectLinks follows every link of the page that points into the subject
	visitSubjectLinks := func(t *testing.T, session *TestSession, page, subject string) {
		escaped := url.PathEscape(subject)
		resp := session.MakeRequest(t, NewRequest(t, "GET", page), http.StatusOK)
		links := NewHTMLParser(t, resp.Body).Find(`a[href^="/subject/"]`)
		visited := 0
		for i := range links.Length() {
			href := links.Eq(i).AttrOr("href", "")
			// decoded, a link into the subject starts with the subject, whether or not it was
			// escaped correctly ("/subject/AC/DC/user2" would be a broken one)
			if decoded, err := url.PathUnescape(strings.TrimPrefix(href, "/subject/")); err == nil && !strings.HasPrefix(decoded, subject) {
				continue // a link to another subject
			}
			assert.True(t, strings.HasPrefix(href, "/subject/"+escaped+"/") || strings.HasPrefix(href, "/subject/"+escaped+"?") ||
				href == "/subject/"+escaped, "%s: subject escaped as one segment: %s", page, href)
			session.MakeRequest(t, NewRequest(t, "GET", href), http.StatusOK)
			visited++
		}
		assert.Positive(t, visited, "%s links to the subject", page)
	}

	for _, subject := range []string{
		"AC/DC",
		"Providence/Stoughton Line",
		"/pol/",
		"Q&A? Yes",
		"100% Pure",
		"1 + 1 = 2",
		"Rock/Pop & Roll? 50%+",
		"Test: Gaudí",
		"When the Pawn...",
	} {
		t.Run(subject, func(t *testing.T) {
			escaped := url.PathEscape(subject)
			repo2 := createArticle(t, token2, subject)
			repo4 := createArticle(t, token4, subject)
			assert.Equal(t, repo2.Name, repo4.Name, "the repository name comes from the slug")

			stored := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{Name: subject})
			assert.Equal(t, repo_model.GenerateSlugFromName(subject), stored.Slug)

			// the API returns the subject as is
			req := NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s", user2.Name, repo2.Name)).AddTokenAuth(token2)
			var apiRepo api.Repository
			DecodeJSON(t, MakeRequest(t, req, http.StatusOK), &apiRepo)
			assert.Equal(t, subject, apiRepo.Subject)

			// the article link escapes the subject as one segment
			repoModel := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo2.ID})
			assert.Equal(t, "/subject/"+escaped+"/user2", repoModel.LinkCtx(t.Context()))

			// subject page, its views, the article page, the compare page
			for _, page := range []string{
				"/subject/" + escaped,
				"/subject/" + escaped + "?view=bubble",
				"/subject/" + escaped + "?view=table",
				"/subject/" + escaped + "?view=article",
				"/subject/" + escaped + "/user2",
				"/subject/" + escaped + "/user4",
				"/subject/" + escaped + "/user2?mode=history",
				"/subject/" + escaped + "/compare/user2...user4",
			} {
				session2.MakeRequest(t, NewRequest(t, "GET", page), http.StatusOK)
			}
			visitSubjectLinks(t, session2, "/subject/"+escaped+"?view=table", subject)
			visitSubjectLinks(t, session2, "/subject/"+escaped+"/user2", subject)
			visitSubjectLinks(t, session2, "/subject/"+escaped+"/compare/user2...user4", subject)

			// Explore lists the subject with a working link
			resp := session2.MakeRequest(t, NewRequest(t, "GET", "/explore/subjects?q="+url.QueryEscape(subject)), http.StatusOK)
			href, ok := NewHTMLParser(t, resp.Body).Find(`a[href="/subject/` + escaped + `"]`).Attr("href")
			require.True(t, ok, "Explore links to %q", subject)
			session2.MakeRequest(t, NewRequest(t, "GET", href), http.StatusOK)

			// create-first-article finds the existing subject from the query string
			session5 := loginUser(t, "user5")
			resp = session5.MakeRequest(t, NewRequest(t, "GET", "/repo/create-first-article?subject="+url.QueryEscape(subject)), http.StatusSeeOther)
			assert.Contains(t, resp.Header().Get("Location"), "/_new/", "create-first-article opens the editor")
			unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: 5, SubjectID: stored.ID})
		})
	}
}

// TestSubjectFirstLetterRedirect covers the MediaWiki-style redirect: new subjects are stored
// with their first letter capitalized, so "/subject/iPhone…" redirects to "/subject/IPhone…",
// keeping the rest of the path and the query string.
func TestSubjectFirstLetterRedirect(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session2 := loginUser(t, "user2")
	session4 := loginUser(t, "user4")
	for _, session := range []*TestSession{session2, session4} {
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)
		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:     "iphone-redirect-test",
			Subject:  "iPhone redirect test",
			AutoInit: true,
			Readme:   "Default",
		}).AddTokenAuth(token)
		var apiRepo api.Repository
		DecodeJSON(t, MakeRequest(t, req, http.StatusCreated), &apiRepo)
		require.Equal(t, "IPhone redirect test", apiRepo.Subject)
	}

	lower, canonical := url.PathEscape("iPhone redirect test"), url.PathEscape("IPhone redirect test")
	for from, to := range map[string]string{
		"/subject/" + lower:                                  "/subject/" + canonical,
		"/subject/" + lower + "?view=table":                  "/subject/" + canonical + "?view=table",
		"/subject/" + lower + "/user2":                       "/subject/" + canonical + "/user2",
		"/subject/" + lower + "/user2?mode=history":          "/subject/" + canonical + "/user2?mode=history",
		"/subject/" + lower + "/user2/issues":                "/subject/" + canonical + "/user2/issues",
		"/subject/" + lower + "/compare/user2...user4":       "/subject/" + canonical + "/compare/user2...user4",
		"/subject/" + url.PathEscape("iPhone_redirect_test"): "/subject/" + canonical, // underscores too
	} {
		resp := session2.MakeRequest(t, NewRequest(t, "GET", from), http.StatusMovedPermanently)
		assert.Equal(t, to, resp.Header().Get("Location"), "redirect of %s", from)
		session2.MakeRequest(t, NewRequest(t, "GET", to), http.StatusOK)
	}

	// no subject under the normalized title either: still a 404
	session2.MakeRequest(t, NewRequest(t, "GET", "/subject/"+url.PathEscape("nothing like this")), http.StatusNotFound)

	// an existing subject is served under its own name, even with a lowercase first letter
	legacy := &repo_model.Subject{Name: "eBay legacy", Slug: repo_model.GenerateSlugFromName("eBay legacy")}
	require.NoError(t, db.Insert(t.Context(), legacy))
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo.SubjectID, repo.SubjectRelation = legacy.ID, nil
	require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "subject_id"))
	session2.MakeRequest(t, NewRequest(t, "GET", "/subject/"+url.PathEscape("eBay legacy")), http.StatusOK)
}
