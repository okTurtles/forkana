// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_CreateModerationTables tests the v335 migration that creates the moderation tables.
func Test_CreateModerationTables(t *testing.T) {
	x, deferable := base.PrepareTestEnv(t, 0)
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	require.NoError(t, CreateModerationTables(x))

	tables, err := x.DBMetas()
	require.NoError(t, err)
	want := map[string][]string{
		"moderation_case":       {"issue_id", "kind", "target_repo_id", "section_anchor", "section_title", "reason", "target_user_id", "decision", "resolution", "needs_admin", "decided_by", "reopened_count", "reopened_unix", "waiting_on", "attention", "remove_scope", "reporter_note", "merged_into_issue_id"},
		"moderation_submission": {"id", "reporter_id", "client_token", "target_repo_id", "commit_sha", "source_start", "source_end", "quote", "section_anchor", "anchor_status"},
		"moderation_report":     {"issue_id", "submission_id", "case_issue_id", "reporter_id", "reason", "verdict", "on_hold"},
		"moderation_audit":      {"id", "created_unix", "actor_id", "actor_role", "action", "case_issue_id", "conversation_issue_id", "target_user_id", "target_repo_id", "reason", "comment_id", "summary"},
	}
	for _, table := range tables {
		cols, ok := want[table.Name]
		if !ok {
			continue
		}
		for _, col := range cols {
			assert.NotNil(t, table.GetColumn(col), "%s.%s should exist", table.Name, col)
		}
		if table.Name == "moderation_case" {
			// the case key: one open case per (article, section, reason) is looked up by it
			var found bool
			for _, idx := range table.Indexes {
				found = found || len(idx.Cols) == 3 && idx.Cols[0] == "target_repo_id" && idx.Cols[1] == "section_anchor" && idx.Cols[2] == "reason"
			}
			assert.True(t, found, "moderation_case has the (target_repo_id, section_anchor, reason) index")
		}
		delete(want, table.Name)
	}
	assert.Empty(t, want, "every moderation table should exist")

	// a reporter's client token is unique, another reporter's same token is not a conflict
	_, err = x.Exec("INSERT INTO moderation_submission (reporter_id, client_token, target_repo_id) VALUES (2, 'tok', 1)")
	assert.NoError(t, err)
	_, err = x.Exec("INSERT INTO moderation_submission (reporter_id, client_token, target_repo_id) VALUES (2, 'tok', 1)")
	assert.Error(t, err)
	_, err = x.Exec("INSERT INTO moderation_submission (reporter_id, client_token, target_repo_id) VALUES (3, 'tok', 1)")
	assert.NoError(t, err)

	// running it again is harmless
	assert.NoError(t, CreateModerationTables(x))
}
