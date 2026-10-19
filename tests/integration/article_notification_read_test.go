// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticleViewMarksRepoNotificationsRead pins that opening an article marks the
// viewer's repository-source notifications for it read (#370), on both the article url
// the notifications link to and the repository url, for a live article and a tombstone,
// and leaves every other notification alone.
func TestArticleViewMarksRepoNotificationsRead(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	owner, repo, subjectName := loadArticleRepo(t, 1)
	require.NotEqual(t, owner.ID, repo.ID)
	other := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	session := loginUser(t, owner.Name)

	insert := func(t *testing.T, userID int64, source activities_model.NotificationSource) *activities_model.Notification {
		t.Helper()
		n := &activities_model.Notification{
			UserID:    userID,
			RepoID:    repo.ID,
			Status:    activities_model.NotificationStatusUnread,
			Source:    source,
			UpdatedBy: other.ID,
		}
		require.NoError(t, db.Insert(t.Context(), n))
		return n
	}
	status := func(t *testing.T, n *activities_model.Notification) activities_model.NotificationStatus {
		t.Helper()
		return unittest.AssertExistsAndLoadBean(t, &activities_model.Notification{ID: n.ID}).Status
	}

	// fixture: an unread issue notification of the owner on this repository
	issue := unittest.AssertExistsAndLoadBean(t, &activities_model.Notification{ID: 4})
	require.Equal(t, owner.ID, issue.UserID)
	require.Equal(t, activities_model.NotificationStatusUnread, issue.Status)

	assertVisitMarksRead := func(t *testing.T, link string) {
		t.Helper()
		pending := insert(t, owner.ID, activities_model.NotificationSourceRepository)
		rejected := insert(t, owner.ID, activities_model.NotificationSourceRepoTransferRejected)
		othersPending := insert(t, other.ID, activities_model.NotificationSourceRepository)

		session.MakeRequest(t, NewRequest(t, "GET", link), http.StatusOK)

		assert.Equal(t, activities_model.NotificationStatusRead, status(t, pending))
		assert.Equal(t, activities_model.NotificationStatusRead, status(t, rejected))
		assert.Equal(t, activities_model.NotificationStatusUnread, status(t, othersPending))
		assert.Equal(t, activities_model.NotificationStatusUnread, status(t, issue))
	}

	articleURL := fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name)
	repoURL := fmt.Sprintf("/%s/%s", owner.Name, repo.Name)
	for _, link := range []string{articleURL, repoURL} {
		t.Run(link, func(t *testing.T) {
			assertVisitMarksRead(t, link)
		})
	}

	t.Run("Tombstone", func(t *testing.T) {
		forkArticle(t, repo)
		req := NewRequestWithValues(t, "POST", repoURL+"/settings",
			deleteForm(GetUserCSRFToken(t, session), owner.Name, subjectName))
		session.MakeRequest(t, req, http.StatusSeeOther)
		require.True(t, unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID}).IsTombstoned)

		// a tombstone is only served at its article url: the repository url redirects there
		resp := session.MakeRequest(t, NewRequest(t, "GET", repoURL), http.StatusSeeOther)
		require.Equal(t, articleURL, test.RedirectURL(resp))
		assertVisitMarksRead(t, articleURL)

		// Without a subject, the tombstone has no article url, and the repository home
		// renders it from a branch that returns before its code view checks.
		_, err := db.GetEngine(t.Context()).ID(repo.ID).Cols("subject_id").Update(&repo_model.Repository{SubjectID: 0})
		require.NoError(t, err)
		assertVisitMarksRead(t, repoURL)
	})
}
