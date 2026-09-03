// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/tests"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExploreSubjects(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Create test subjects
	subject1, err := repo_model.GetOrCreateSubject(t.Context(), "Test Subject Alpha")
	assert.NoError(t, err)
	assert.NotNil(t, subject1)

	subject2, err := repo_model.GetOrCreateSubject(t.Context(), "Test Subject Beta")
	assert.NoError(t, err)
	assert.NotNil(t, subject2)

	// Test basic page load
	req := NewRequest(t, "GET", "/explore/subjects")
	resp := MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, http.StatusOK, resp.Code)

	// Test search functionality
	req = NewRequest(t, "GET", "/explore/subjects?q=Alpha")
	resp = MakeRequest(t, req, http.StatusOK)
	respStr := resp.Body.String()
	assert.Contains(t, respStr, `<input type="search" name="q" value="Alpha"`)

	// Test sorting: the requested sort is the one marked as selected in the sort menu
	req = NewRequest(t, "GET", "/explore/subjects?sort=alphabetically")
	resp = MakeRequest(t, req, http.StatusOK)
	respStr = resp.Body.String()
	assert.Contains(t, respStr, `checked value="alphabetically"`)

	// Test pagination
	req = NewRequest(t, "GET", "/explore/subjects?page=1")
	resp = MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestExploreSubjectsSorting(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Test all sort options the subjects list offers
	sortOptions := []string{
		"alphabetically",
		"reversealphabetically",
		"recentupdate",
		"leastupdate",
		"mostforks",
		"fewestforks",
		"mostcontributors",
		"fewestcontributors",
	}

	for _, sortType := range sortOptions {
		req := NewRequest(t, "GET", "/explore/subjects?sort="+sortType)
		resp := MakeRequest(t, req, http.StatusOK)
		assert.Equal(t, http.StatusOK, resp.Code, "Sort type %s should work", sortType)
	}
}

func TestExploreSubjectSuggestions(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	for _, name := range []string{"Moon", "Moons of Saturn", "Full Moon Party", "Sun"} {
		subject, err := repo_model.GetOrCreateSubject(t.Context(), name)
		assert.NoError(t, err)
		assert.NotNil(t, subject)
	}

	suggestPath := func(t *testing.T, path string) []string {
		t.Helper()
		req := NewRequest(t, "GET", path)
		resp := MakeRequest(t, req, http.StatusOK)
		var parsed struct {
			Subjects []string `json:"subjects"`
		}
		DecodeJSON(t, resp, &parsed)
		return parsed.Subjects
	}
	suggest := func(t *testing.T, keyword string) []string {
		t.Helper()
		return suggestPath(t, "/explore/subjects/suggestions?q="+url.QueryEscape(keyword))
	}

	// The exact match comes first, then the subjects starting with the keyword, then the rest.
	assert.Equal(t, []string{"Moon", "Moons of Saturn", "Full Moon Party"}, suggest(t, "moon"))

	// Only matching subjects are suggested, and a keyword without matches suggests nothing.
	assert.Equal(t, []string{"Sun"}, suggest(t, "sun"))
	assert.Empty(t, suggest(t, "nothing matches this"))

	// An empty keyword, with or without the parameter itself, suggests nothing.
	assert.Empty(t, suggest(t, ""))
	assert.Empty(t, suggestPath(t, "/explore/subjects/suggestions"))
}

// TestExploreSubjectsListMarkup locks the name-only subject row from #248. The row design was
// lost once already because it lived in a template the Explore page had stopped rendering, and
// nothing asserted on the markup, so the regression was invisible to CI.
func TestExploreSubjectsListMarkup(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	subject, err := repo_model.GetOrCreateSubject(t.Context(), "Markup Probe Subject")
	assert.NoError(t, err)
	assert.NotNil(t, subject)

	req := NewRequest(t, "GET", "/explore/subjects")
	html := MakeRequest(t, req, http.StatusOK).Body.String()

	// The name links through AppSubUrl, so the page survives a sub-path deployment.
	assert.Contains(t, html, `href="`+setting.AppSubURL+`/subject/Markup%20Probe%20Subject"`)

	// The leading glyph is the target that matches the Subjects tab, not the stock book. The
	// substring holds whether or not the SVG set is loaded, since the fallback for a missing
	// icon still spells the icon name out.
	assert.Contains(t, html, "octicon-goal")
	assert.NotContains(t, html, "octicon-book")

	// Neither the stock repository counts nor the created/updated line belong in the row.
	assert.NotContains(t, html, "flex-item-trailing")
	assert.NotContains(t, html, "octicon-repo-forked")
}

// TestExploreArticlesRemoved pins the removal of the unreachable /explore/articles listing.
// Nothing in the UI ever linked to it; the history route below shares the prefix and stays.
func TestExploreArticlesRemoved(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	MakeRequest(t, NewRequest(t, "GET", "/explore/articles"), http.StatusNotFound)
	MakeRequest(t, NewRequest(t, "GET", "/explore/articles?q=test"), http.StatusNotFound)
	MakeRequest(t, NewRequest(t, "GET", "/explore/articles/sitemap-1.xml"), http.StatusNotFound)

	// The article history view is a different route and must keep working.
	MakeRequest(t, NewRequest(t, "GET", "/explore/articles/history/user2/repo1"), http.StatusOK)
}

// TestExploreSubjectsSitemap pins the sitemap that moved off /explore/articles. It has to be XML
// (the subjects sitemap route used to fall through to the HTML page) and it has to list article
// URLs, which is exactly what crawlers were fed from the old path.
func TestExploreSubjectsSitemap(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	resp := MakeRequest(t, NewRequest(t, "GET", "/explore/subjects/sitemap-1.xml"), http.StatusOK)
	assert.Equal(t, "text/xml", resp.Header().Get("Content-Type"))

	body := resp.Body.String()
	assert.Contains(t, body, "<urlset")
	assert.Contains(t, body, "<loc>"+setting.AppURL+"article/user2/example-subject</loc>")

	// The sitemap index advertises the subjects path, not the removed articles one.
	index := MakeRequest(t, NewRequest(t, "GET", "/sitemap.xml"), http.StatusOK).Body.String()
	assert.Contains(t, index, setting.AppURL+"explore/subjects/sitemap-1.xml")
	assert.NotContains(t, index, "explore/articles/sitemap-")
}

// TestExploreSubjectsExactMatchNotHiddenByFilters covers #319: searching for a subject that
// already exists used to offer "Want to create it?" whenever the active filters hid it from the
// exact-match lookup. The "not a fork" filter does exactly that for any subject that already has
// a fork, and accepting the offer routes through GetOrCreateSubject, which returns the existing
// subject and attaches yet another article to it.
func TestExploreSubjectsExactMatchNotHiddenByFilters(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	const subjectName = "Fork Filter Probe"
	subject, err := repo_model.GetOrCreateSubject(t.Context(), subjectName)
	require.NoError(t, err)

	// A root article plus a fork of it, so that the "fork=0" filter excludes the subject.
	require.NoError(t, db.Insert(t.Context(), &repo_model.Repository{
		OwnerID: 2, OwnerName: "user2",
		Name: "fork-filter-probe-root", LowerName: "fork-filter-probe-root",
		SubjectID: subject.ID, IsFork: false,
	}))
	require.NoError(t, db.Insert(t.Context(), &repo_model.Repository{
		OwnerID: 3, OwnerName: "user3",
		Name: "fork-filter-probe-fork", LowerName: "fork-filter-probe-fork",
		SubjectID: subject.ID, IsFork: true,
	}))

	path := "/explore/subjects?fork=0&q=" + url.QueryEscape(subjectName)
	createSelector := `a[href^="` + setting.AppSubURL + `/repo/create?subject="]`

	subjectHrefs := func(h *HTMLDoc) []string {
		hrefs := make([]string, 0)
		h.Find(`a[href^="` + setting.AppSubURL + `/subject/"]`).Each(func(_ int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			hrefs = append(hrefs, href)
		})
		return hrefs
	}

	// Signed out.
	resp := MakeRequest(t, NewRequest(t, "GET", path), http.StatusOK)
	anonDoc := NewHTMLParser(t, resp.Body)
	AssertHTMLElement(t, anonDoc, createSelector, false)

	// Signed in.
	session := loginUser(t, "user2")
	resp = session.MakeRequest(t, NewRequest(t, "GET", path), http.StatusOK)
	authDoc := NewHTMLParser(t, resp.Body)
	AssertHTMLElement(t, authDoc, createSelector, false)

	// explore.Subjects never reads ctx.Doer, so the two responses must list the same subjects in
	// the same order. This pins the "signed-in users see fewer subjects" hypothesis as ruled out.
	assert.Equal(t, subjectHrefs(anonDoc), subjectHrefs(authDoc))

	// Positive control: the offer is still made for a name that really is free, so the assertions
	// above cannot pass just because the selector stopped matching anything.
	resp = MakeRequest(t, NewRequest(t, "GET", "/explore/subjects?fork=0&q="+url.QueryEscape("No Such Subject Here")), http.StatusOK)
	AssertHTMLElement(t, NewHTMLParser(t, resp.Body), createSelector, true)
}
