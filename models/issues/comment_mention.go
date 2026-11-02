// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"strings"

	"code.gitea.io/gitea/models/db"
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

// GetLatestMentionComments returns, for each of the issues, its most recent comment
// that @-mentions the user named username. Issues where only the description mentions
// the user (or nothing does any more, after an edit) are absent from the map. The
// comments' posters are loaded.
func GetLatestMentionComments(ctx context.Context, issueIDs []int64, username string) (map[int64]*Comment, error) {
	res := make(map[int64]*Comment, len(issueIDs))
	if len(issueIDs) == 0 || username == "" {
		return res, nil
	}

	comments := make(CommentList, 0, len(issueIDs))
	if err := db.GetEngine(ctx).
		Where(builder.In("issue_id", issueIDs)).
		And(builder.Eq{"type": CommentTypeComment}).
		And(db.BuildCaseInsensitiveLike("content", "@"+username)).
		OrderBy("created_unix DESC, id DESC").
		Find(&comments); err != nil {
		return nil, err
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
