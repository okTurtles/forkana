// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

func TestExploreRepos(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	req := NewRequest(t, "GET", "/explore/articles?q=TheKeyword&topic=1&language=TheLang")
	resp := MakeRequest(t, req, http.StatusOK)
	respStr := resp.Body.String()

	assert.Contains(t, respStr, `<input type="hidden" name="topic" value="true">`)
	assert.Contains(t, respStr, `<input type="hidden" name="language" value="TheLang">`)
	assert.Contains(t, respStr, `<input type="search" name="q" value="TheKeyword"`)
}

func TestExploreReposBlankKeyword(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// The landing page search field submits to the explore page with relevance ordering, so a
	// blank keyword reaches the article list with "sort=score" and has to be handled there
	// instead of failing the search query (#257).
	for _, keyword := range []string{"", "%20", "+", ",", "%20,%20,%20"} {
		for _, sortOrder := range []string{"score", "reversescore", ""} {
			t.Run("q="+keyword+"&sort="+sortOrder, func(t *testing.T) {
				req := NewRequest(t, "GET", "/explore/articles?only_show_relevant=false&fork=0&sort="+sortOrder+"&q="+keyword)
				MakeRequest(t, req, http.StatusOK)

				req = NewRequest(t, "GET", "/explore/subjects?only_show_relevant=false&fork=0&sort="+sortOrder+"&q="+keyword)
				MakeRequest(t, req, http.StatusOK)
			})
		}
	}
}
