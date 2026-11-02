// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package issues_test

import (
	"testing"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	"code.gitea.io/gitea/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMentionsUser(t *testing.T) {
	assert.True(t, issues_model.MentionsUser("@user2 please have a look", "user2"))
	assert.True(t, issues_model.MentionsUser("agreed with @User2.", "user2"))
	assert.False(t, issues_model.MentionsUser("@user20 please have a look", "user2"))
	assert.False(t, issues_model.MentionsUser("mail me at me@user2.org", "user2"))
	assert.False(t, issues_model.MentionsUser("nobody here", "user2"))
	assert.False(t, issues_model.MentionsUser("@user2", ""))
}

func TestGetLatestMentionComments(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	insert := func(id, issueID, posterID, created int64, content string, typ issues_model.CommentType) {
		require.NoError(t, db.Insert(t.Context(), &issues_model.Comment{
			ID: id, Type: typ, IssueID: issueID, PosterID: posterID, Content: content,
		}))
		// xorm stamps created_unix with the current time on insert: set the one the test needs
		_, err := db.GetEngine(t.Context()).Exec("UPDATE comment SET created_unix = ? WHERE id = ?", created, id)
		require.NoError(t, err)
	}
	insert(9001, 2, 1, 100, "older: @user2 have a look", issues_model.CommentTypeComment)
	insert(9002, 2, 4, 200, "newer: @user2, what do you think?", issues_model.CommentTypeComment)
	insert(9003, 2, 5, 300, "@user20 is someone else", issues_model.CommentTypeComment)
	insert(9004, 3, 4, 400, "@user2 on a code line", issues_model.CommentTypeCode)
	insert(9005, 11, 5, 500, "cc @user2", issues_model.CommentTypeComment)

	got, err := issues_model.GetLatestMentionComments(t.Context(), []int64{2, 3, 11}, "user2")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.EqualValues(t, 9002, got[2].ID)
	assert.EqualValues(t, 4, got[2].Poster.ID)
	assert.EqualValues(t, 9005, got[11].ID)
	assert.NotContains(t, got, int64(3)) // a code comment is not a conversation comment

	got, err = issues_model.GetLatestMentionComments(t.Context(), nil, "user2")
	require.NoError(t, err)
	assert.Empty(t, got)
}
