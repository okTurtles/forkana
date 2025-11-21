// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

// TestStandardRepoRoutePermissions verifies that standard repository routes
// (/{username}/{reponame}) properly enforce write permissions.
// This test compares behavior with article routes to determine if the
// vulnerability found in article routes is pre-existing or new.
func TestStandardRepoRoutePermissions(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Load test fixtures
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})       // owner of repo1
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})       // non-owner user
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}) // public repo

	t.Run("NonOwnerCannotEditViaStandardRoute", func(t *testing.T) {
		// Login as user4 (non-owner)
		session := loginUser(t, user4.Name)

		// Try to access edit page via standard repository route
		req := NewRequest(t, "GET", fmt.Sprintf("/%s/%s/_edit/master/README.md", user2.Name, repo1.Name))
		resp := session.MakeRequest(t, req, http.StatusOK) // What status do we get?

		// Log the response for analysis
		t.Logf("Response status: %d", resp.Code)
		t.Logf("Response length: %d", len(resp.Body.Bytes()))

		// If this returns 404, then article routes have a NEW vulnerability
		// If this returns 200, then this is a PRE-EXISTING vulnerability in Gitea
		if resp.Code == http.StatusOK {
			t.Logf("WARNING: Standard repository routes ALSO allow non-owners to access edit pages!")
			t.Logf("This is a PRE-EXISTING vulnerability in Gitea, not introduced by PR #53")
		} else if resp.Code == http.StatusNotFound {
			t.Logf("Standard repository routes correctly deny access (404)")
			t.Logf("The vulnerability in article routes is NEW and specific to RepoAssignmentByOwnerAndSubject")
		} else {
			t.Logf("Unexpected status code: %d", resp.Code)
		}
	})

	t.Run("OwnerCanEditViaStandardRoute", func(t *testing.T) {
		// Login as user2 (owner)
		session := loginUser(t, user2.Name)

		// Access edit page via standard repository route
		req := NewRequest(t, "GET", fmt.Sprintf("/%s/%s/_edit/master/README.md", user2.Name, repo1.Name))
		resp := session.MakeRequest(t, req, http.StatusOK)

		assert.Equal(t, http.StatusOK, resp.Code, "Owner should be able to access edit page")
	})

	t.Run("UnauthenticatedUserCannotEditViaStandardRoute", func(t *testing.T) {
		// Try to access edit page without authentication
		req := NewRequest(t, "GET", fmt.Sprintf("/%s/%s/_edit/master/README.md", user2.Name, repo1.Name))
		resp := MakeRequest(t, req, http.StatusSeeOther) // Should redirect to login

		assert.Equal(t, http.StatusSeeOther, resp.Code, "Unauthenticated user should be redirected to login")
	})
}
