// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArticleViewMarksRepoNotificationsRead pins that opening an article marks the
// viewer's repository-source notifications for it read (#370), on both the article url
// the notifications link to and the repository url, and leaves every other notification
// alone. The viewer and repository ids differ, so swapped arguments would not match.
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

	for _, link := range []string{
		fmt.Sprintf("/subject/%s/%s", subjectName, owner.Name),
		fmt.Sprintf("/%s/%s", owner.Name, repo.Name),
	} {
		t.Run(link, func(t *testing.T) {
			pending := insert(t, owner.ID, activities_model.NotificationSourceRepository)
			rejected := insert(t, owner.ID, activities_model.NotificationSourceRepoTransferRejected)
			othersPending := insert(t, other.ID, activities_model.NotificationSourceRepository)
			// fixture: an unread issue notification of the owner on this repository
			issue := unittest.AssertExistsAndLoadBean(t, &activities_model.Notification{ID: 4})
			require.Equal(t, owner.ID, issue.UserID)
			require.Equal(t, activities_model.NotificationStatusUnread, issue.Status)

			session.MakeRequest(t, NewRequest(t, "GET", link), http.StatusOK)

			assert.Equal(t, activities_model.NotificationStatusRead, status(t, pending))
			assert.Equal(t, activities_model.NotificationStatusRead, status(t, rejected))
			assert.Equal(t, activities_model.NotificationStatusUnread, status(t, othersPending))
			assert.Equal(t, activities_model.NotificationStatusUnread, status(t, issue))
		})
	}
}
