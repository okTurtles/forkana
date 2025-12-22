// Copyright 2017 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	org_model "code.gitea.io/gitea/models/organization"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	api "code.gitea.io/gitea/modules/structs"
	org_service "code.gitea.io/gitea/services/org"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateForkNoLogin(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/user2/repo1/forks", &api.CreateForkOption{})
	MakeRequest(t, req, http.StatusUnauthorized)
}

func TestAPIForkListLimitedAndPrivateRepos(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user1Sess := loginUser(t, "user1")
	user1 := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user1"})

	// fork into a limited org
	limitedOrg := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 22})
	assert.Equal(t, api.VisibleTypeLimited, limitedOrg.Visibility)

	ownerTeam1, err := org_model.OrgFromUser(limitedOrg).GetOwnerTeam(t.Context())
	assert.NoError(t, err)
	assert.NoError(t, org_service.AddTeamMember(t.Context(), ownerTeam1, user1))
	user1Token := getTokenForLoggedInUser(t, user1Sess, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteOrganization)
	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/user2/repo1/forks", &api.CreateForkOption{
		Organization: &limitedOrg.Name,
	}).AddTokenAuth(user1Token)
	MakeRequest(t, req, http.StatusAccepted)

	// fork into a private org
	user4Sess := loginUser(t, "user4")
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user4"})
	privateOrg := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 23})
	assert.Equal(t, api.VisibleTypePrivate, privateOrg.Visibility)

	ownerTeam2, err := org_model.OrgFromUser(privateOrg).GetOwnerTeam(t.Context())
	assert.NoError(t, err)
	assert.NoError(t, org_service.AddTeamMember(t.Context(), ownerTeam2, user4))
	user4Token := getTokenForLoggedInUser(t, user4Sess, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteOrganization)
	req = NewRequestWithJSON(t, "POST", "/api/v1/repos/user2/repo1/forks", &api.CreateForkOption{
		Organization: &privateOrg.Name,
	}).AddTokenAuth(user4Token)
	MakeRequest(t, req, http.StatusAccepted)

	t.Run("Anonymous", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1/forks")
		resp := MakeRequest(t, req, http.StatusOK)

		var forks []*api.Repository
		DecodeJSON(t, resp, &forks)

		assert.Empty(t, forks)
		assert.Equal(t, "0", resp.Header().Get("X-Total-Count"))
	})

	t.Run("Logged in", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1/forks").AddTokenAuth(user1Token)
		resp := MakeRequest(t, req, http.StatusOK)

		var forks []*api.Repository
		DecodeJSON(t, resp, &forks)

		assert.Len(t, forks, 2)
		assert.Equal(t, "2", resp.Header().Get("X-Total-Count"))

		assert.NoError(t, org_service.AddTeamMember(t.Context(), ownerTeam2, user1))

		req = NewRequest(t, "GET", "/api/v1/repos/user2/repo1/forks").AddTokenAuth(user1Token)
		resp = MakeRequest(t, req, http.StatusOK)

		forks = []*api.Repository{}
		DecodeJSON(t, resp, &forks)

		assert.Len(t, forks, 2)
		assert.Equal(t, "2", resp.Header().Get("X-Total-Count"))
	})
}

func TestGetPrivateReposForks(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user1Sess := loginUser(t, "user1")
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2}) // private repository
	privateOrg := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 23})
	user1Token := getTokenForLoggedInUser(t, user1Sess, auth_model.AccessTokenScopeWriteRepository)

	forkedRepoName := "forked-repo"
	// create fork from a private repository
	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/"+repo2.FullName()+"/forks", &api.CreateForkOption{
		Organization: &privateOrg.Name,
		Name:         &forkedRepoName,
	}).AddTokenAuth(user1Token)
	MakeRequest(t, req, http.StatusAccepted)

	// test get a private fork without clear permissions
	req = NewRequest(t, "GET", "/api/v1/repos/"+repo2.FullName()+"/forks").AddTokenAuth(user1Token)
	resp := MakeRequest(t, req, http.StatusOK)

	forks := []*api.Repository{}
	DecodeJSON(t, resp, &forks)
	assert.Len(t, forks, 1)
	assert.Equal(t, "1", resp.Header().Get("X-Total-Count"))
	assert.Equal(t, "forked-repo", forks[0].Name)
	assert.Equal(t, privateOrg.Name, forks[0].Owner.UserName)
}

// TestAPIForkWithSubjectConflict tests that forking a repository fails with HTTP 403
// when the user already owns a different repository for the same subject.
// This is a Forkana-specific constraint: each user can only have one repository per subject.
func TestAPIForkWithSubjectConflict(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// Setup:
	// - user2 owns repo1 with subject_id: 1 (example-subject)
	// - user5 will create their own repo for the same subject
	// - user5 then tries to fork repo1 - should fail with 403

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	// Verify repo1 has a subject
	require.Positive(t, repo1.SubjectID, "repo1 should have a subject")
	subject := unittest.AssertExistsAndLoadBean(t, &repo_model.Subject{ID: repo1.SubjectID})

	// Create a repository for user5 with the same subject
	// In Forkana, this creates a fork of the root repository
	user5Repo, err := repo_service.CreateRepositoryDirectly(t.Context(), user5, user5, repo_service.CreateRepoOptions{
		Name:    "my-subject-article",
		Subject: subject.Name,
	}, true)
	require.NoError(t, err)
	require.NotNil(t, user5Repo)
	t.Cleanup(func() {
		if user5Repo != nil {
			_ = repo_service.DeleteRepositoryDirectly(t.Context(), user5Repo.ID)
		}
	})

	// Get API token for user5
	user5Sess := loginUser(t, user5.Name)
	user5Token := getTokenForLoggedInUser(t, user5Sess, auth_model.AccessTokenScopeWriteRepository)

	t.Run("ForkBlockedBySubjectOwnership", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		// user5 tries to fork repo1 (owned by user2) for the same subject
		// This should fail because user5 already owns a repo for this subject
		req := NewRequestWithJSON(t, "POST", "/api/v1/repos/"+user2.Name+"/"+repo1.Name+"/forks", &api.CreateForkOption{}).
			AddTokenAuth(user5Token)
		resp := MakeRequest(t, req, http.StatusForbidden)

		// Verify the error message mentions subject ownership
		var apiError api.APIError
		DecodeJSON(t, resp, &apiError)
		assert.Contains(t, apiError.Message, "subject",
			"Error message should mention subject ownership conflict")
	})

	t.Run("ForkSucceedsForUserWithoutSubjectRepo", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		// user4 doesn't own a repo for the same subject as repo1
		// so they should be able to fork repo1 successfully
		user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
		user4Sess := loginUser(t, user4.Name)
		user4Token := getTokenForLoggedInUser(t, user4Sess, auth_model.AccessTokenScopeWriteRepository)

		// user4 should be able to fork repo1 since they don't own a repo for that subject
		forkName := "user4-fork-of-repo1"
		req := NewRequestWithJSON(t, "POST", "/api/v1/repos/"+user2.Name+"/"+repo1.Name+"/forks", &api.CreateForkOption{
			Name: &forkName,
		}).AddTokenAuth(user4Token)
		resp := MakeRequest(t, req, http.StatusAccepted)

		var fork api.Repository
		DecodeJSON(t, resp, &fork)
		assert.Equal(t, forkName, fork.Name)
		assert.Equal(t, user4.Name, fork.Owner.UserName)

		// Clean up the fork
		defer func() {
			forkRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{Name: forkName, OwnerID: user4.ID})
			_ = repo_service.DeleteRepositoryDirectly(t.Context(), forkRepo.ID)
		}()
	})

	t.Run("ForkBlockedWithCustomName", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		// Even with a custom fork name, the fork should be blocked
		// because user5 already owns a repo for the same subject
		customName := "custom-fork-name"
		req := NewRequestWithJSON(t, "POST", "/api/v1/repos/"+user2.Name+"/"+repo1.Name+"/forks", &api.CreateForkOption{
			Name: &customName,
		}).AddTokenAuth(user5Token)
		MakeRequest(t, req, http.StatusForbidden)
	})
}
