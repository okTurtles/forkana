// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
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
