// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"strings"
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
	req := NewRequest(t, "GET", "/explore/articles")
	resp := MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, http.StatusOK, resp.Code)

	// Test search functionality
	req = NewRequest(t, "GET", "/explore/articles?q=Alpha")
	resp = MakeRequest(t, req, http.StatusOK)
	respStr := resp.Body.String()
	assert.Contains(t, respStr, `value="Alpha"`)

	// Test sorting
	req = NewRequest(t, "GET", "/explore/articles?sort=alphabetically")
	resp = MakeRequest(t, req, http.StatusOK)
	respStr = resp.Body.String()
	assert.Contains(t, respStr, `value="alphabetically"`)

	// Test pagination
	req = NewRequest(t, "GET", "/explore/articles?page=1")
	resp = MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestExploreSubjectsSorting(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Test all sort options
	sortOptions := []string{
		"alphabetically",
		"reversealphabetically",
		"newest",
		"oldest",
		"recentupdate",
		"leastupdate",
	}

	for _, sortType := range sortOptions {
		req := NewRequest(t, "GET", "/explore/articles?sort="+sortType)
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

// TestExploreNavbarActiveTab locks the explore navbar markup from #294. The tab the page belongs
// to must carry the "active" class and no other tab may, and the tabs must live inside the
// ".overflow-menu-items" wrapper: the <overflow-menu> web component waits for that element before
// it initialises, and the CSS that aligns the active tab's underline with the menu rail is keyed
// on it too. Without the wrapper the tab still renders, but unstyled and never collapsing.
func TestExploreNavbarActiveTab(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// activeTab returns the href of the one active tab together with the parsed page, so the
	// caller can make further assertions about the same response instead of fetching it again.
	activeTab := func(path string) (string, *HTMLDoc) {
		req := NewRequest(t, "GET", path)
		resp := MakeRequest(t, req, http.StatusOK)
		h := NewHTMLParser(t, resp.Body)

		// exactly one tab is active, and it is inside the overflow-menu wrapper
		active := h.Find(`overflow-menu .overflow-menu-items a.item.active`)
		assert.Equal(t, 1, active.Length(), "exactly one explore tab should be active on %s", path)
		href, exists := active.Attr("href")
		assert.True(t, exists, "the active explore tab should be a link on %s", path)
		return href, h
	}

	subjectsHref := setting.AppSubURL + "/explore/subjects"
	usersHref := setting.AppSubURL + "/explore/users"

	// the tab the page belongs to is the active one, and the others are not
	subjectsTab, subjectsPage := activeTab("/explore/subjects?q=mars")
	assert.True(t, strings.HasPrefix(subjectsTab, subjectsHref),
		"the Subjects tab should be the active one on /explore/subjects")
	usersTab, _ := activeTab("/explore/users?q=mars")
	assert.True(t, strings.HasPrefix(usersTab, usersHref),
		"the Users tab should be the active one on /explore/users")

	// the inactive tabs are still rendered, just not marked active. The "still rendered" half is
	// what keeps the pair from passing vacuously against a tab that stopped rendering at all.
	// Only the Users tab gets this pair: the Code tab is gated on "IsRepoIndexerEnabled", which
	// explore.Subjects never puts in the template data, so it never renders here and asserting it
	// is not active would be trivially true. The "exactly one active tab" check above already
	// covers every other tab anyway.
	assert.Equal(t, 1, subjectsPage.Find(`overflow-menu .overflow-menu-items a.item[href^="`+usersHref+`"]`).Length(),
		"the Users tab should still be rendered on /explore/subjects")
	assert.Equal(t, 0, subjectsPage.Find(`overflow-menu .overflow-menu-items a.item.active[href^="`+usersHref+`"]`).Length(),
		"the Users tab must not be active on /explore/subjects")
}
