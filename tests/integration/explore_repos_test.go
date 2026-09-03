// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	"code.gitea.io/gitea/tests"
)

// TestRepoSearchBlankKeyword locks the #257 regression: a blank keyword submitted with relevance
// ordering used to leave the "relevance_score" placeholder in the ORDER BY clause and fail the
// query with a 500. The fix lives in repo_model.SearchRepository, so it is exercised through the
// two surviving callers: the subjects list, and the admin repository list (RenderRepoSearch).
func TestRepoSearchBlankKeyword(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user1") // admin, for the admin repository list

	for _, keyword := range []string{"", "%20", "+", ",", "%20,%20,%20"} {
		for _, sortOrder := range []string{"score", "reversescore", ""} {
			t.Run("q="+keyword+"&sort="+sortOrder, func(t *testing.T) {
				query := "only_show_relevant=false&fork=0&sort=" + sortOrder + "&q=" + keyword

				req := NewRequest(t, "GET", "/explore/subjects?"+query)
				MakeRequest(t, req, http.StatusOK)

				req = NewRequest(t, "GET", "/-/admin/repos?"+query)
				session.MakeRequest(t, req, http.StatusOK)
			})
		}
	}
}
