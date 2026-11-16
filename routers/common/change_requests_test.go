// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"testing"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #532: the navbar's "Change requests" counter is CounterLabel primary ("something new")
// while one of the open change requests it counts has an unread notification for the user.
func TestHasUnreadReceivedChangeRequests(t *testing.T) {
	require.NoError(t, unittest.LoadFixtures())
	ctx := t.Context()
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	setStatus := func(t *testing.T, notificationID int64, status activities_model.NotificationStatus) {
		_, err := db.GetEngine(ctx).ID(notificationID).Cols("status").Update(&activities_model.Notification{Status: status})
		require.NoError(t, err)
	}
	hasUnread := func(t *testing.T, user *user_model.User) bool {
		unread, err := HasUnreadReceivedChangeRequests(ctx, user)
		require.NoError(t, err)
		return unread
	}

	// user2 owns repo1, which receives the open change requests 2, 3 and 11. Its fixture
	// notifications: change request 2 read, change request 3 pinned, and issue 5 (closed,
	// not a change request) unread.
	assert.False(t, hasUnread(t, user2), "a read or pinned notification is nothing new, nor an unread issue")

	// a new comment on change request 2 makes its notification unread again
	setStatus(t, 2, activities_model.NotificationStatusUnread)
	assert.True(t, hasUnread(t, user2))

	// someone else's unread notifications don't count: user1 has an unread one on issue 1
	user1 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	assert.False(t, hasUnread(t, user1))

	// once the change request is closed it is no longer counted, nor is its notification
	_, err := db.GetEngine(ctx).Exec("UPDATE issue SET is_closed = ? WHERE id = ?", true, 2)
	require.NoError(t, err)
	assert.False(t, hasUnread(t, user2))
	_, err = db.GetEngine(ctx).Exec("UPDATE issue SET is_closed = ? WHERE id = ?", false, 2)
	require.NoError(t, err)

	// read again, nothing new
	setStatus(t, 2, activities_model.NotificationStatusRead)
	assert.False(t, hasUnread(t, user2))
}
