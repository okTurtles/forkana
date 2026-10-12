// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"strings"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/tests"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The endpoints of the article editor's link and cite tools (issue #399).
// Fixtures: user2's repo1 is the only article of the subject "example-subject".
func TestCitationEditorEndpoints(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	t.Run("SuggestionDetails", func(t *testing.T) {
		resp := MakeRequest(t, NewRequest(t, "GET", "/explore/subjects/suggestions?detail=1&q=example-subject"), http.StatusOK)
		var parsed struct {
			Subjects []string `json:"subjects"`
			Details  []struct {
				Name     string `json:"name"`
				Articles int64  `json:"articles"`
			} `json:"details"`
		}
		DecodeJSON(t, resp, &parsed)
		if assert.NotEmpty(t, parsed.Details) && assert.NotEmpty(t, parsed.Subjects) {
			assert.Equal(t, "example-subject", parsed.Details[0].Name)
			assert.EqualValues(t, 1, parsed.Details[0].Articles)
			assert.Equal(t, parsed.Subjects[0], parsed.Details[0].Name)
		}
	})

	t.Run("SubjectArticles", func(t *testing.T) {
		resp := MakeRequest(t, NewRequest(t, "GET", "/explore/subjects/articles?subject=example-subject"), http.StatusOK)
		var parsed struct {
			Subject  string `json:"subject"`
			Articles []struct {
				Owner string `json:"owner"`
			} `json:"articles"`
		}
		DecodeJSON(t, resp, &parsed)
		assert.Equal(t, "example-subject", parsed.Subject)
		if assert.Len(t, parsed.Articles, 1) {
			assert.Equal(t, "user2", parsed.Articles[0].Owner)
		}

		resp = MakeRequest(t, NewRequest(t, "GET", "/explore/subjects/articles?subject=no-such-subject"), http.StatusOK)
		assert.JSONEq(t, `{"articles":[]}`, resp.Body.String())
		// an empty name is no subject (not the first subject of the table)
		resp = MakeRequest(t, NewRequest(t, "GET", "/explore/subjects/articles?subject="), http.StatusOK)
		assert.JSONEq(t, `{"articles":[]}`, resp.Body.String())
	})

	t.Run("PrivateOwnersAreHidden", func(t *testing.T) {
		// a public repository of a private organization is not listed, counted or linked
		subject, err := repo_model.GetOrCreateSubject(t.Context(), "Private org subject")
		require.NoError(t, err)
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 40}) // privated_org, public
		repo.SubjectID = subject.ID
		require.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(t.Context(), repo, "subject_id"))

		resp := MakeRequest(t, NewRequest(t, "GET", "/explore/subjects/articles?subject=Private+org+subject"), http.StatusOK)
		assert.JSONEq(t, `{"subject":"Private org subject","articles":[]}`, resp.Body.String())
		resp = MakeRequest(t, NewRequest(t, "GET", "/explore/subjects/suggestions?detail=1&q=Private+org+subject"), http.StatusOK)
		assert.Contains(t, resp.Body.String(), `{"name":"Private org subject","articles":0}`)
	})

	t.Run("LookupIsASignedInPost", func(t *testing.T) {
		// a GET (e.g. an <img> on another site) can't trigger a fetch
		MakeRequest(t, NewRequest(t, "GET", "/-/citation/lookup?q=10.1126/science.167.3918.449"), http.StatusMethodNotAllowed)
		session := loginUser(t, "user2")
		csrf := GetUserCSRFToken(t, session)
		// nor a POST without the CSRF token
		session.MakeRequest(t, NewRequestWithValues(t, "POST", "/-/citation/lookup", map[string]string{"q": "just words"}), http.StatusBadRequest)
		resp := session.MakeRequest(t, NewRequestWithValues(t, "POST", "/-/citation/lookup", map[string]string{
			"_csrf": csrf,
			"q":     "just words",
		}), http.StatusBadRequest)
		assert.Contains(t, resp.Body.String(), "not a URL, DOI or ISBN")
	})

	t.Run("ArticleRendersLinksAndReferences", func(t *testing.T) {
		// the markdown preview renders the way an article does
		session := loginUser(t, "user2")
		req := NewRequestWithValues(t, "POST", "/-/markup", map[string]string{
			"_csrf":     GetUserCSRFToken(t, session),
			"mode":      "file",
			"context":   "/user2/repo1/src/branch/master",
			"file_path": "README.md",
			"text": "See [example-subject](subject/example-subject) and [Chang'e 4](subject/Chang'e 4).[^1]\n\n" +
				"[^1]: [\"Luna 2\"](https://nssdc.gsfc.nasa.gov/x). *NASA–NSSDC*.\n",
		})
		resp := session.MakeRequest(t, req, http.StatusOK)
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(resp.Body.String()))
		assert.NoError(t, err)
		link := doc.Find("a.internal-link").First()
		assert.Equal(t, "example-subject", link.Text())
		assert.True(t, strings.HasSuffix(link.AttrOr("href", ""), "/subject/example-subject")) // the preview writes absolute links
		missing := doc.Find("a.internal-link-missing")
		assert.Equal(t, "Chang'e 4", missing.Text())
		assert.True(t, strings.HasSuffix(missing.AttrOr("href", ""), "/repo/create?subject=Chang%27e+4"))
		assert.Equal(t, "[1]", doc.Find("sup.footnote-ref").Text())
		assert.Equal(t, "References", doc.Find(".footnotes h2.footnotes-heading").Text())
		assert.Equal(t, 1, doc.Find(".footnotes a.external-link").Length())
	})
}
