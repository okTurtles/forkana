// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"strings"
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
			// Scope the assertions to the header element so unrelated page copy cannot affect them.
			desc := NewHTMLParser(t, resp.Body).Find("#pull-desc-display").Text()
			assert.Contains(t, desc, c.contains)
			for _, absent := range c.absent {
				assert.NotContains(t, desc, absent)
			}
		})
	}
}

// TestPullViewChangeRequestTitleDescPlural checks that a single-change request is not described
// as "1 changes".
func TestPullViewChangeRequestTitleDescPlural(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// /user2/repo1/pulls/5 carries a single commit; user2 owns the article and user1 posted it.
	cases := []struct {
		user     string
		contains string
	}{
		{user: "user2", contains: "1 change to your article"},
		{user: "user1", contains: "You want to make 1 change to the article"},
		{user: "user4", contains: "1 change to the article"},
	}

	for _, c := range cases {
		t.Run(c.user, func(t *testing.T) {
			session := loginUser(t, c.user)
			req := NewRequest(t, "GET", "/user2/repo1/pulls/5")
			resp := session.MakeRequest(t, req, http.StatusOK)
			desc := strings.Join(strings.Fields(NewHTMLParser(t, resp.Body).Find("#pull-desc-display").Text()), " ")
			assert.Contains(t, desc, c.contains)
			assert.NotContains(t, desc, "1 changes")
		})
	}
}

// TestPullViewMergePermissionHint checks that the upstream "only those with write access ... can
// merge change requests" hint is gone: merging is owner-only in Forkana, so that copy contradicted
// the owner-scoped message shown right below it to every non-owner viewer.
func TestPullViewMergePermissionHint(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	const writeAccessHint = "can merge change requests"

	// user2 owns repo1 and can merge, user40 has write access but is not the owner,
	// user4 has neither.
	cases := []struct {
		user   string
		absent []string
		// contains is the accurate, Forkana-specific message the viewer should get instead.
		contains string
	}{
		{user: "user2", absent: []string{writeAccessHint, "Only the article owner can accept change requests."}},
		{user: "user40", absent: []string{writeAccessHint}, contains: "Only the article owner can accept change requests."},
		{user: "user4", absent: []string{writeAccessHint}, contains: "You are not authorized to merge this change request."},
	}

	for _, c := range cases {
		t.Run(c.user, func(t *testing.T) {
			session := loginUser(t, c.user)
			req := NewRequest(t, "GET", "/user2/repo1/pulls/3")
			resp := session.MakeRequest(t, req, http.StatusOK)
			body := resp.Body.String()
			for _, absent := range c.absent {
				assert.NotContains(t, body, absent)
			}
			if c.contains != "" {
				assert.Contains(t, body, c.contains)
			}
		})
	}
}
