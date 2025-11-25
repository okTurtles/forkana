// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

// TestArticleRoutePermissions tests that article routes correctly enforce permissions
// This addresses GEMINI-6 security concern from PR #53 review
func TestArticleRoutePermissions(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Load test fixtures
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})       // owner of repo1 (public)
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})       // non-owner user
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}) // public repo with subject_id: 1

	// Load subject for repo1
	err := repo1.LoadSubject(t.Context())
	assert.NoError(t, err)
	if repo1.SubjectRelation == nil {
		t.Skip("Repo1 doesn't have a subject, skipping test")
		return
	}

	subjectName := repo1.SubjectRelation.Name

	t.Run("UnauthenticatedUserCanViewPublicArticle", func(t *testing.T) {
		// Unauthenticated users should be able to view public articles
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", user2.Name, subjectName))
		resp := MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), subjectName)
	})

	t.Run("UnauthenticatedUserCannotEditArticle", func(t *testing.T) {
		// Unauthenticated users should not be able to access edit routes
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_edit/master/README.md", user2.Name, subjectName))
		MakeRequest(t, req, http.StatusNotFound) // Should redirect to login or return 404
	})

	t.Run("AuthenticatedNonOwnerCanViewPublicArticle", func(t *testing.T) {
		// Authenticated non-owner should be able to view public articles
		session := loginUser(t, user4.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), subjectName)
	})

	t.Run("AuthenticatedNonOwnerCannotEditArticle", func(t *testing.T) {
		// Authenticated non-owner without write permission should not be able to edit
		session := loginUser(t, user4.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_edit/master/README.md", user2.Name, subjectName))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("OwnerCanViewOwnArticle", func(t *testing.T) {
		// Repository owner should be able to view their own article
		session := loginUser(t, user2.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), subjectName)
	})

	t.Run("OwnerCanEditOwnArticle", func(t *testing.T) {
		// Repository owner should be able to access edit routes
		session := loginUser(t, user2.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_edit/master/README.md", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), "README.md")
	})
}

// TestArticleRoutePrivateRepository tests permissions for private repositories
func TestArticleRoutePrivateRepository(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Load test fixtures
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})       // owner of repo2 (private)
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})       // non-owner user
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2}) // private repo

	// Create a subject for repo2 if it doesn't have one
	if repo2.SubjectID == 0 {
		t.Skip("Repo2 doesn't have a subject, skipping private repo test")
		return
	}

	err := repo2.LoadSubject(t.Context())
	assert.NoError(t, err)
	assert.NotNil(t, repo2.SubjectRelation)

	subjectName := repo2.SubjectRelation.Name

	t.Run("UnauthenticatedUserCannotViewPrivateArticle", func(t *testing.T) {
		// Unauthenticated users should not be able to view private articles
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", user2.Name, subjectName))
		MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("NonCollaboratorCannotViewPrivateArticle", func(t *testing.T) {
		// Authenticated non-collaborator should not be able to view private articles
		session := loginUser(t, user4.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", user2.Name, subjectName))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("OwnerCanViewPrivateArticle", func(t *testing.T) {
		// Repository owner should be able to view their own private article
		session := loginUser(t, user2.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), subjectName)
	})
}

// TestArticleRouteNonExistentSubject tests error handling for non-existent subjects
func TestArticleRouteNonExistentSubject(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	t.Run("NonExistentSubjectReturns404", func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/nonexistent-subject-12345", user2.Name))
		MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("NonExistentUserReturns404", func(t *testing.T) {
		req := NewRequest(t, "GET", "/article/nonexistentuser12345/some-subject")
		MakeRequest(t, req, http.StatusNotFound)
	})
}

// TestArticleFileOperationPermissions tests file operation routes (edit, upload, delete)
func TestArticleFileOperationPermissions(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	err := repo1.LoadSubject(t.Context())
	assert.NoError(t, err)
	if repo1.SubjectRelation == nil {
		t.Skip("Repo1 doesn't have a subject, skipping test")
		return
	}

	subjectName := repo1.SubjectRelation.Name

	t.Run("NonOwnerCannotCreateFile", func(t *testing.T) {
		session := loginUser(t, user4.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_new/master/", user2.Name, subjectName))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("OwnerCanCreateFile", func(t *testing.T) {
		session := loginUser(t, user2.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_new/master/", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), "New File")
	})

	t.Run("NonOwnerCannotUploadFile", func(t *testing.T) {
		session := loginUser(t, user4.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_upload/master/", user2.Name, subjectName))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("OwnerCanUploadFile", func(t *testing.T) {
		session := loginUser(t, user2.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_upload/master/", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), "Upload")
	})

	t.Run("NonOwnerCannotDeleteFile", func(t *testing.T) {
		session := loginUser(t, user4.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_delete/master/README.md", user2.Name, subjectName))
		session.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("OwnerCanDeleteFile", func(t *testing.T) {
		session := loginUser(t, user2.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_delete/master/README.md", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Contains(t, resp.Body.String(), "Delete")
	})
}

// TestArticleRouteMiddlewareChain verifies the middleware chain is correctly applied
func TestArticleRouteMiddlewareChain(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	err := repo1.LoadSubject(t.Context())
	assert.NoError(t, err)
	if repo1.SubjectRelation == nil {
		t.Skip("Repo1 doesn't have a subject, skipping test")
		return
	}

	subjectName := repo1.SubjectRelation.Name

	t.Run("RepoAssignmentByOwnerAndSubjectPopulatesContext", func(t *testing.T) {
		// This test verifies that the middleware correctly populates ctx.Repo
		session := loginUser(t, user2.Name)
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s", user2.Name, subjectName))
		resp := session.MakeRequest(t, req, http.StatusOK)

		// The page should render correctly, which means ctx.Repo was populated
		assert.Contains(t, resp.Body.String(), subjectName)
		assert.Contains(t, resp.Body.String(), user2.Name)
	})

	t.Run("PermissionChecksAreEnforced", func(t *testing.T) {
		// Verify that permission checks prevent unauthorized access
		session := loginUser(t, user2.Name)

		// Create a token with limited scope (read-only)
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeReadRepository)

		// Try to edit with read-only token - should fail
		req := NewRequest(t, "GET", fmt.Sprintf("/article/%s/%s/_edit/master/README.md", user2.Name, subjectName)).
			AddTokenAuth(token)
		MakeRequest(t, req, http.StatusNotFound) // Should be denied due to insufficient permissions
	})
}

// TestArticleRouteWithEmptyRepository tests handling of empty repositories
func TestArticleRouteWithEmptyRepository(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Find an empty repository in fixtures or skip
	// This test ensures empty repos are handled gracefully
	t.Run("EmptyRepoHandling", func(t *testing.T) {
		// TODO: Add test for empty repository once we have a fixture with an empty repo + subject
		t.Skip("Need to add empty repository fixture with subject")
	})
}
