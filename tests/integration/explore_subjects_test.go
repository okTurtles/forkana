// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
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

// TestExploreSubjectsNoDefaultSortSelected covers #292: the sort dropdown used to paint its
// default entry ("Most recently updated") in the static grey active state before the user
// had chosen anything, because the handler echoed its internal ordering default back to the
// template. Nothing may look selected until the user selects it.
func TestExploreSubjectsNoDefaultSortSelected(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	req := NewRequest(t, "GET", "/explore/subjects")
	resp := MakeRequest(t, req, http.StatusOK)
	h := NewHTMLParser(t, resp.Body)
	assert.Equal(t, 0, h.Find(`.menu label.active.item`).Length(), "no sort entry may be active before the user picks one")
	assert.Equal(t, 0, h.Find(`.menu input[name="sort"][checked]`).Length(), "no sort radio may be checked before the user picks one")

	// Once a sort is explicitly requested, that entry -- and only that entry -- is active.
	req = NewRequest(t, "GET", "/explore/subjects?sort=alphabetically")
	resp = MakeRequest(t, req, http.StatusOK)
	h = NewHTMLParser(t, resp.Body)
	active := h.Find(`.menu label.active.item`)
	assert.Equal(t, 1, active.Length())
	value, exists := active.Find(`input[name="sort"]`).Attr("value")
	assert.True(t, exists)
	assert.Equal(t, "alphabetically", value)
	assert.Equal(t, 1, h.Find(`.menu input[name="sort"][checked]`).Length())
}
