// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"strings"
	"testing"

	"code.gitea.io/gitea/tests"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
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
		if assert.NotEmpty(t, parsed.Details) {
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
	})

	t.Run("LookupNeedsSignIn", func(t *testing.T) {
		MakeRequest(t, NewRequest(t, "GET", "/-/citation/lookup?q=10.1126/science.167.3918.449"), http.StatusSeeOther)
		session := loginUser(t, "user2")
		resp := session.MakeRequest(t, NewRequest(t, "GET", "/-/citation/lookup?q=just+words"), http.StatusBadRequest)
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
