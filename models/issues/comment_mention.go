// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"slices"
	"strings"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/references"

	"xorm.io/builder"
)

// MentionsUser reports whether content @-mentions the user named username, the way
// the mention notifications read it (references.FindAllMentionsMarkdown): "@user2"
// mentions user2, "@user20" and "email@user2.org" do not.
func MentionsUser(content, username string) bool {
	if username == "" || !strings.Contains(strings.ToLower(content), "@"+strings.ToLower(username)) {
		return false
	}
	for _, name := range references.FindAllMentionsMarkdown(content) {
		if strings.EqualFold(name, username) {
			return true
		}
	}
	return false
}

// GetLatestMentionComments returns, for each of the issues, its most recent comment,
// review or code comment that @-mentions the user named username; pending reviews,
// private to their author, are left out. Issues where only the description mentions
// the user (or nothing does any more, after an edit) are absent from the map. The
// comments' posters are loaded. The LIKE only pre-filters (it also matches "@user20"),
// so the latest exact mention per issue is picked here rather than in SQL; the callers
// pass one page of issues.
func GetLatestMentionComments(ctx context.Context, issueIDs []int64, username string) (map[int64]*Comment, error) {
	res := make(map[int64]*Comment, len(issueIDs))
	if len(issueIDs) == 0 || username == "" {
		return res, nil
	}

	// in chunks that stay under the databases' parameter limits, like issueIDsWhere; an
	// issue is never split across chunks, so the first match per issue is still its latest
	var comments CommentList
	for chunk := range slices.Chunk(issueIDs, db.DefaultMaxInSize) {
		var got CommentList
		if err := db.GetEngine(ctx).
			Where(builder.In("issue_id", chunk)).
			// review bodies and code comments mention too (services/pull/review.go)
			And(builder.In("type", CommentTypeComment, CommentTypeReview, CommentTypeCode)).
			And(builder.Or(
				builder.Eq{"review_id": 0},
				builder.NotIn("review_id", builder.Select("id").From("review").Where(builder.Eq{"type": ReviewTypePending})),
			)).
			And(db.BuildCaseInsensitiveLike("content", "@"+username)).
			OrderBy("created_unix DESC, id DESC").
			Find(&got); err != nil {
			return nil, err
		}
		comments = append(comments, got...)
	}

	latest := make(CommentList, 0, len(issueIDs))
	for _, c := range comments {
		if _, ok := res[c.IssueID]; ok || !MentionsUser(c.Content, username) {
			continue
		}
		res[c.IssueID] = c
		latest = append(latest, c)
	}
	if err := latest.LoadPosters(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

// GetMergedPullIssueIDs returns which of the issues are merged pull requests, reading
// only that flag, in chunks that stay under the databases' parameter limits.
func GetMergedPullIssueIDs(ctx context.Context, issueIDs []int64) (container.Set[int64], error) {
	return issueIDsWhere(ctx, "pull_request", "issue_id", issueIDs, builder.Eq{"has_merged": true})
}

// GetClosedIssueIDs returns which of the issues are closed, reading only that flag, in
// chunks that stay under the databases' parameter limits.
func GetClosedIssueIDs(ctx context.Context, issueIDs []int64) (container.Set[int64], error) {
	return issueIDsWhere(ctx, "issue", "id", issueIDs, builder.Eq{"is_closed": true})
}

func issueIDsWhere(ctx context.Context, table, idCol string, issueIDs []int64, cond builder.Cond) (container.Set[int64], error) {
	res := make(container.Set[int64], len(issueIDs))
	for chunk := range slices.Chunk(issueIDs, db.DefaultMaxInSize) {
		var got []int64
		if err := db.GetEngine(ctx).Table(table).Cols(idCol).
			Where(builder.In(idCol, chunk)).And(cond).
			Find(&got); err != nil {
			return nil, err
		}
		res.AddMultiple(got...)
	}
	return res, nil
}
