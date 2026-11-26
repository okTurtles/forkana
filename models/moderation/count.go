// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"

	"xorm.io/builder"
)

// CountOpenAssignedCases counts the open cases (type/case issues of the cases repository)
// assigned to the user: the badge beside the account menu's Moderation entry. It is one query,
// because the account menu is on every page. Subqueries rather than joins, so that an issue
// is counted once whatever its labels and assignees. A case waiting on its owner after Request
// sources still counts (foundations decisions 12).
func CountOpenAssignedCases(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, nil
	}
	return db.GetEngine(ctx).Table("issue").
		Where(builder.Eq{"issue.is_closed": false, "issue.is_pull": false}).
		And(builder.In("issue.repo_id", repo_model.ModerationCasesRepoIDCond())).
		And(builder.In("issue.id", builder.Select("issue_id").From("issue_assignees").
			Where(builder.Eq{"assignee_id": userID}))).
		And(builder.In("issue.id", builder.Select("issue_label.issue_id").From("issue_label").
			InnerJoin("label", "label.id = issue_label.label_id").
			Where(builder.Eq{"label.name": LabelTypeCase}.And(builder.In("label.org_id", repo_model.ModerationOrgIDCond()))))).
		Count()
}
