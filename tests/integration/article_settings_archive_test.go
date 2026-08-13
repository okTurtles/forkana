// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"code.gitea.io/gitea/models/perm"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/test"
	repo_service "code.gitea.io/gitea/services/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadArticleRepo returns the owner, the repository and its subject name, skipping
// the test when the fixture has no subject attached.
func loadArticleRepo(t *testing.T, repoID int64) (*user_model.User, *repo_model.Repository, string) {
	t.Helper()
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})

	require.NoError(t, repo.LoadSubject(t.Context()))
	if repo.SubjectRelation == nil {
		t.Skipf("repo %d has no subject, skipping", repoID)
	}
	return owner, repo, repo.SubjectRelation.Name
}

// archiveForm builds the payload the article archive modal submits.
func archiveForm(csrf, owner, subject string) map[string]string {
	return map[string]string{
		"_csrf":               csrf,
		"action":              "archive",
		"redirect_to_article": "true",
		"article_name":        owner + "/" + subject,
	}
}

func TestArticleSettingsArchiveSuccess(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	assert.False(t, repo.IsArchived)

	session := loginUser(t, owner.Name)
	settingsURL := fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name)

	req := NewRequestWithValues(t, "POST", settingsURL,
		archiveForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
	resp := session.MakeRequest(t, req, http.StatusSeeOther)

	articleSettingsURL := fmt.Sprintf("/article/%s/%s?view=article&mode=settings", owner.Name, subjectName)
	assert.Equal(t, articleSettingsURL, test.RedirectURL(resp))

	// the flash message is carried over in a cookie, so it renders on the next page
	req = NewRequest(t, "GET", articleSettingsURL)
	resp = session.MakeRequest(t, req, http.StatusOK)
	htmlDoc := NewHTMLParser(t, resp.Body)

	flash := htmlDoc.Find(".flash-message")
	require.Equal(t, 1, flash.Length())
	assert.Contains(t, flash.Text(), "This article has been archived.")

	archived := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	assert.True(t, archived.IsArchived)
}

func TestArticleSettingsArchiveUnauthorized(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	settingsURL := fmt.Sprintf("/%s/%s/settings", owner.Name, repo.Name)

	t.Run("NonCollaborator", func(t *testing.T) {
		// blocked before the handler runs, by the "settings" group admin requirement
		session := loginUser(t, user4.Name)
		req := NewRequestWithValues(t, "POST", settingsURL,
			archiveForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
		session.MakeRequest(t, req, http.StatusNotFound)

		unarchived := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.False(t, unarchived.IsArchived)
	})

	t.Run("AdminCollaboratorIsNotOwner", func(t *testing.T) {
		// an admin collaborator passes the group requirement but is rejected by the
		// owner-only guard in the archive handler
		require.NoError(t, repo_service.AddOrUpdateCollaborator(t.Context(), repo, user4, perm.AccessModeAdmin))

		session := loginUser(t, user4.Name)
		req := NewRequestWithValues(t, "POST", settingsURL,
			archiveForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
		session.MakeRequest(t, req, http.StatusForbidden)

		unarchived := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
		assert.False(t, unarchived.IsArchived)
	})
}
