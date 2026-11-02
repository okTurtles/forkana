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

func TestGetMergedAndClosedIssueIDs(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	// issue 2 is a merged pull request (pull_request.yml); issue 5 is closed (issue.yml)
	ids := []int64{1, 2, 3, 5, 11}
	merged, err := issues_model.GetMergedPullIssueIDs(t.Context(), ids)
	require.NoError(t, err)
	assert.True(t, merged.Contains(2))
	assert.False(t, merged.Contains(3))
	closed, err := issues_model.GetClosedIssueIDs(t.Context(), ids)
	require.NoError(t, err)
	assert.True(t, closed.Contains(5))
	assert.False(t, closed.Contains(3))

	empty, err := issues_model.GetClosedIssueIDs(t.Context(), nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestGetLatestMentionComments(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	insert := func(id, issueID, posterID, created int64, content string, typ issues_model.CommentType, reviewID int64) {
		require.NoError(t, db.Insert(t.Context(), &issues_model.Comment{
			ID: id, Type: typ, IssueID: issueID, PosterID: posterID, Content: content, ReviewID: reviewID,
		}))
		// xorm stamps created_unix with the current time on insert: set the one the test needs
		_, err := db.GetEngine(t.Context()).Exec("UPDATE comment SET created_unix = ? WHERE id = ?", created, id)
		require.NoError(t, err)
	}
	// a pending review is private to its author; a submitted one is not
	require.NoError(t, db.Insert(t.Context(), &issues_model.Review{ID: 9101, Type: issues_model.ReviewTypePending, ReviewerID: 4, IssueID: 3}))
	require.NoError(t, db.Insert(t.Context(), &issues_model.Review{ID: 9102, Type: issues_model.ReviewTypeComment, ReviewerID: 5, IssueID: 12}))

	insert(9001, 2, 1, 100, "older: @user2 have a look", issues_model.CommentTypeComment, 0)
	insert(9002, 2, 4, 200, "newer: @user2, what do you think?", issues_model.CommentTypeComment, 0)
	insert(9003, 2, 5, 300, "@user20 is someone else", issues_model.CommentTypeComment, 0)
	insert(9004, 3, 4, 400, "@user2 in a pending review", issues_model.CommentTypeCode, 9101)
	insert(9005, 11, 5, 500, "cc @user2", issues_model.CommentTypeComment, 0)
	insert(9006, 12, 5, 600, "@user2 on a code line", issues_model.CommentTypeCode, 9102)
	insert(9007, 11, 4, 700, "@user2 in a review body", issues_model.CommentTypeReview, 0)
	insert(9008, 13, 4, 800, "@user2 in a label change", issues_model.CommentTypeLabel, 0)

	got, err := issues_model.GetLatestMentionComments(t.Context(), []int64{2, 3, 11, 12, 13}, "user2")
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.EqualValues(t, 9002, got[2].ID)
	assert.EqualValues(t, 4, got[2].Poster.ID)
	assert.EqualValues(t, 9007, got[11].ID, "a review body mentions")
	assert.EqualValues(t, 9006, got[12].ID, "a code comment of a submitted review mentions")
	assert.EqualValues(t, 5, got[12].Poster.ID)
	assert.NotContains(t, got, int64(3), "a pending review is private to its author")
	assert.NotContains(t, got, int64(13))

	got, err = issues_model.GetLatestMentionComments(t.Context(), nil, "user2")
	require.NoError(t, err)
	assert.Empty(t, got)
}
