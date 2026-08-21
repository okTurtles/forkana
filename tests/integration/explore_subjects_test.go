// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
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
