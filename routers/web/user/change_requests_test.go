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
	// every state ticked stays ticked (searchChangeRequests reads it as no filter)
	assert.Equal(t, []string{"open", "merged", "closed"}, parseChangeRequestStates("closed,merged,open"))
	assert.Equal(t, []string{"open", "closed"}, parseChangeRequestStates("closed,bogus,open"))
	assert.Equal(t, []string{"merged"}, parseChangeRequestStates("merged"))

	assert.Equal(t, "open,merged", toggleChangeRequestState([]string{"open"}, "merged"))
	assert.Empty(t, toggleChangeRequestState([]string{"open"}, "open"))
	assert.Equal(t, "closed", toggleChangeRequestState([]string{"merged", "closed"}, "merged"))
	assert.Equal(t, "open,merged,closed", toggleChangeRequestState([]string{"open", "merged"}, "closed"))
	assert.Equal(t, "open,closed", toggleChangeRequestState([]string{"open", "merged", "closed"}, "merged"))
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
		// user5 mentions user2 in a code comment on change request 11
		require.NoError(t, db.Insert(t.Context(), &issues_model.IssueUser{UID: 2, IssueID: 11, IsMentioned: true}))
		require.NoError(t, db.Insert(t.Context(), &issues_model.Comment{
			Type: issues_model.CommentTypeCode, IssueID: 11, PosterID: 5, Content: "@user2 is this line right?", TreePath: "README.md", Line: 1,
		}))
		// change request 12's description mentions user2; change request 2's mention was edited away
		_, err := db.GetEngine(t.Context()).Exec("UPDATE issue SET content = ? WHERE id = ?", "cc @user2 for the dates", 12)
		require.NoError(t, err)
		require.NoError(t, db.Insert(t.Context(), &issues_model.IssueUser{UID: 2, IssueID: 12, IsMentioned: true}))
		require.NoError(t, db.Insert(t.Context(), &issues_model.IssueUser{UID: 2, IssueID: 2, IsMentioned: true}))

		data := changeRequestsPage(t, 2, map[string]string{"tab": "mentions"})
		rows := map[int64]*ChangeRequestRow{}
		for _, row := range data["Rows"].([]*ChangeRequestRow) {
			rows[row.Issue.ID] = row
		}
		require.Len(t, rows, 4)
		assert.EqualValues(t, 1, rows[3].Who.ID)
		assert.Equal(t, "@user2 could you check the dates?", rows[3].Quote)
		assert.EqualValues(t, 5, rows[11].Who.ID, "a code comment's author is the one who mentioned")
		assert.Equal(t, "@user2 is this line right?", rows[11].Quote)
		assert.Equal(t, rows[12].Issue.PosterID, rows[12].Who.ID, "the description's author")
		assert.Equal(t, "cc @user2 for the dates", rows[12].Quote)
		assert.Equal(t, rows[2].Issue.PosterID, rows[2].Who.ID)
		assert.Empty(t, rows[2].Quote, "no quote when nothing mentions the user any more")
		assert.EqualValues(t, 4, data["TabCounts"].(map[string]int64)[common.ChangeRequestsMentions])
	})

	t.Run("All states ticked", func(t *testing.T) {
		all := changeRequestsPage(t, 2, map[string]string{"state": "open,merged,closed"})
		assert.Equal(t, []string{"open", "merged", "closed"}, all["SelectedStates"])
		assert.Equal(t, "open,merged,closed", all["StateFilter"])
		assert.Equal(t, rowIssueIDs(changeRequestsPage(t, 2, nil)), rowIssueIDs(all))
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
		count, err := common.CountOpenReceivedChangeRequests(ctx, ctx.Doer)
		require.NoError(t, err)
		data := changeRequestsPage(t, 2, nil)
		assert.Equal(t, data["TabCounts"].(map[string]int64)[common.ChangeRequestsReceived], count)
		assert.Equal(t, count, data["ChangeRequestsHubReceivedCount"])
		assert.Positive(t, count)
	})
}
