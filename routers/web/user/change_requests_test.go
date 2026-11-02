// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"net/http"
	"testing"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/routers/common"
	"code.gitea.io/gitea/services/contexttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChangeRequestStates(t *testing.T) {
	assert.Nil(t, parseChangeRequestStates(""))
	assert.Nil(t, parseChangeRequestStates("open,closed,merged"), "every state is no filter")
	assert.Equal(t, []string{"open", "closed"}, parseChangeRequestStates("closed,bogus,open"))
	assert.Equal(t, []string{"merged"}, parseChangeRequestStates("merged"))

	assert.Equal(t, "open,merged", toggleChangeRequestState([]string{"open"}, "merged"))
	assert.Empty(t, toggleChangeRequestState([]string{"open"}, "open"))
	assert.Equal(t, "closed", toggleChangeRequestState([]string{"merged", "closed"}, "merged"))
}

func changeRequestsPage(t *testing.T, userID int64, form map[string]string) map[string]any {
	ctx, _ := contexttest.MockContext(t, "pulls")
	contexttest.LoadUser(t, ctx, userID)
	for k, v := range form {
		ctx.Req.Form.Set(k, v)
	}
	ChangeRequests(ctx)
	require.Equal(t, http.StatusOK, ctx.Resp.WrittenStatus())
	return ctx.Data
}

func rowIssueIDs(data map[string]any) []int64 {
	var ids []int64
	for _, row := range data["Rows"].([]*ChangeRequestRow) {
		ids = append(ids, row.Issue.ID)
	}
	return ids
}

func TestChangeRequestsHub(t *testing.T) {
	defer test.MockVariableValue(&setting.UI.IssuePagingNum, 20)()
	require.NoError(t, unittest.LoadFixtures())

	t.Run("Received", func(t *testing.T) {
		// user2 owns repo1, where user1 opened change requests 2, 3 and 11
		data := changeRequestsPage(t, 2, nil)
		assert.Equal(t, common.ChangeRequestsReceived, data["Tab"])
		assert.Equal(t, true, data["PageIsChangeRequestsHub"])
		ids := rowIssueIDs(data)
		assert.Subset(t, ids, []int64{2, 3, 11})
		for _, row := range data["Rows"].([]*ChangeRequestRow) {
			assert.True(t, row.Issue.IsPull)
			assert.Equal(t, row.Issue.PosterID, row.Who.ID, "Received rows lead with the author")
			assert.NotEmpty(t, row.Article)
		}
		counts := data["TabCounts"].(map[string]int64)
		assert.Positive(t, counts[common.ChangeRequestsReceived])
	})

	t.Run("Sent", func(t *testing.T) {
		data := changeRequestsPage(t, 2, map[string]string{"tab": "sent"})
		assert.Equal(t, common.ChangeRequestsSent, data["Tab"])
		rows := data["Rows"].([]*ChangeRequestRow)
		require.NotEmpty(t, rows)
		for _, row := range rows {
			assert.EqualValues(t, 2, row.Issue.PosterID)
			assert.Equal(t, row.Issue.Repo.OwnerID, row.Who.ID, "Sent rows lead with the article's owner")
		}
		// Gitea's dashboard filter still lands on the matching tab
		assert.Equal(t, common.ChangeRequestsSent, changeRequestsPage(t, 2, map[string]string{"type": "created_by"})["Tab"])
	})

	t.Run("Mentions", func(t *testing.T) {
		// user1 mentions user2 on change request 3 (repo1)
		require.NoError(t, db.Insert(t.Context(), &issues_model.IssueUser{UID: 2, IssueID: 3, IsMentioned: true}))
		require.NoError(t, db.Insert(t.Context(), &issues_model.Comment{
			Type: issues_model.CommentTypeComment, IssueID: 3, PosterID: 1, Content: "@user2 **could you** check the dates?",
		}))
		data := changeRequestsPage(t, 2, map[string]string{"tab": "mentions"})
		rows := data["Rows"].([]*ChangeRequestRow)
		require.Len(t, rows, 1)
		assert.EqualValues(t, 3, rows[0].Issue.ID)
		assert.EqualValues(t, 1, rows[0].Who.ID)
		assert.Equal(t, "@user2 could you check the dates?", rows[0].Quote)
		assert.EqualValues(t, 1, data["TabCounts"].(map[string]int64)[common.ChangeRequestsMentions])
	})

	t.Run("States", func(t *testing.T) {
		// change request 2 has been merged
		_, err := db.GetEngine(t.Context()).Exec("UPDATE issue SET is_closed = ? WHERE id = ?", true, 2)
		require.NoError(t, err)
		// change request 11 is closed without being merged
		_, err = db.GetEngine(t.Context()).Exec("UPDATE issue SET is_closed = ? WHERE id = ?", true, 11)
		require.NoError(t, err)

		merged := rowIssueIDs(changeRequestsPage(t, 2, map[string]string{"state": "merged"}))
		assert.Contains(t, merged, int64(2))
		assert.NotContains(t, merged, int64(11))
		assert.NotContains(t, merged, int64(3))

		closed := rowIssueIDs(changeRequestsPage(t, 2, map[string]string{"state": "closed"}))
		assert.Contains(t, closed, int64(11))
		assert.NotContains(t, closed, int64(2))

		openOrMerged := changeRequestsPage(t, 2, map[string]string{"state": "open,merged"})
		ids := rowIssueIDs(openOrMerged)
		assert.Contains(t, ids, int64(2))
		assert.Contains(t, ids, int64(3))
		assert.NotContains(t, ids, int64(11))
		assert.EqualValues(t, len(ids), openOrMerged["Total"])

		open := rowIssueIDs(changeRequestsPage(t, 2, map[string]string{"state": "open"}))
		assert.Contains(t, open, int64(3))
		assert.NotContains(t, open, int64(2))
		assert.NotContains(t, open, int64(11))

		for _, row := range changeRequestsPage(t, 2, nil)["Rows"].([]*ChangeRequestRow) {
			switch row.Issue.ID {
			case 2:
				assert.Equal(t, ChangeRequestMerged, row.State)
			case 11:
				assert.Equal(t, ChangeRequestClosed, row.State)
			case 3:
				assert.Equal(t, ChangeRequestOpen, row.State)
			}
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		defer test.MockVariableValue(&setting.UI.IssuePagingNum, 1)()
		first := changeRequestsPage(t, 2, map[string]string{"state": "open,merged"})
		total := first["Total"].(int64)
		require.GreaterOrEqual(t, total, int64(2))
		assert.Len(t, first["Rows"], 1)
		assert.Equal(t, 1, first["ShownUpTo"])
		second := changeRequestsPage(t, 2, map[string]string{"state": "open,merged", "page": "2"})
		assert.Len(t, second["Rows"], 1)
		assert.Equal(t, 2, second["ShownUpTo"])
		assert.NotEqual(t, rowIssueIDs(first), rowIssueIDs(second))
	})

	t.Run("Navbar counter", func(t *testing.T) {
		ctx, _ := contexttest.MockContext(t, "/")
		contexttest.LoadUser(t, ctx, 2)
		count, err := common.CountOpenChangeRequests(ctx, ctx.Doer, common.ChangeRequestsReceived)
		require.NoError(t, err)
		assert.Equal(t, changeRequestsPage(t, 2, nil)["TabCounts"].(map[string]int64)[common.ChangeRequestsReceived], count)
		assert.Positive(t, count)
	})
}
