// Copyright 2025 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

// TestPullViewChangeRequestTitleDesc checks that the change request header line is viewer-aware:
// the change author reads it in the first person, the article owner keeps the possessive form,
// and any other viewer gets the neutral form.
func TestPullViewChangeRequestTitleDesc(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// user1 is the poster of /user2/repo1/pulls/3, user2 owns the article, user4 is neither.
	cases := []struct {
		user     string
		contains string
		absent   []string
	}{
		{user: "user1", contains: "You want to make", absent: []string{"wants to make"}},
		{user: "user2", contains: "changes to your article", absent: []string{"You want to make", "changes to the article"}},
		{user: "user4", contains: "changes to the article", absent: []string{"You want to make", "changes to your article"}},
	}

	for _, c := range cases {
		t.Run(c.user, func(t *testing.T) {
			session := loginUser(t, c.user)
			req := NewRequest(t, "GET", "/user2/repo1/pulls/3")
			resp := session.MakeRequest(t, req, http.StatusOK)
			body := resp.Body.String()
			assert.Contains(t, body, c.contains)
			for _, absent := range c.absent {
				assert.NotContains(t, body, absent)
			}
		})
	}
}

// TestPullViewMergePermissionHint checks that the "only those with write access ... can merge"
// hint is hidden from the article owner, who is the one person able to merge.
func TestPullViewMergePermissionHint(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	const hint = "can merge change requests"

	session := loginUser(t, "user2") // owner of repo1, can merge
	req := NewRequest(t, "GET", "/user2/repo1/pulls/3")
	resp := session.MakeRequest(t, req, http.StatusOK)
	assert.NotContains(t, resp.Body.String(), hint)

	session = loginUser(t, "user4") // cannot merge
	req = NewRequest(t, "GET", "/user2/repo1/pulls/3")
	resp = session.MakeRequest(t, req, http.StatusOK)
	assert.Contains(t, resp.Body.String(), hint)
}
