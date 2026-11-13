// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
)

// Test_CreateModerationTables tests the v335 migration that creates the moderation tables.
func Test_CreateModerationTables(t *testing.T) {
	x, deferable := base.PrepareTestEnv(t, 0)
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	assert.NoError(t, CreateModerationTables(x))

	tables, err := x.DBMetas()
	assert.NoError(t, err)
	want := map[string][]string{
		"moderation_case":       {"issue_id", "kind", "target_repo_id", "target_user_id", "decision", "resolution", "needs_admin", "decided_by", "reopened_count", "reopened_unix", "waiting_on_owner"},
		"moderation_submission": {"id", "reporter_id", "client_token", "case_issue_id", "commit_sha", "source_start", "source_end", "quote", "section_anchor", "anchor_status"},
		"moderation_report":     {"issue_id", "submission_id", "case_issue_id", "reporter_id", "reason", "verdict", "verdict_note", "on_hold"},
		"moderation_audit":      {"id", "created_unix", "actor_id", "actor_role", "action", "case_issue_id", "summary"},
	}
	for _, table := range tables {
		cols, ok := want[table.Name]
		if !ok {
			continue
		}
		for _, col := range cols {
			assert.NotNil(t, table.GetColumn(col), "%s.%s should exist", table.Name, col)
		}
		delete(want, table.Name)
	}
	assert.Empty(t, want, "every moderation table should exist")

	// a reporter's client token is unique, another reporter's same token is not a conflict
	_, err = x.Exec("INSERT INTO moderation_submission (reporter_id, client_token, case_issue_id) VALUES (2, 'tok', 1)")
	assert.NoError(t, err)
	_, err = x.Exec("INSERT INTO moderation_submission (reporter_id, client_token, case_issue_id) VALUES (2, 'tok', 1)")
	assert.Error(t, err)
	_, err = x.Exec("INSERT INTO moderation_submission (reporter_id, client_token, case_issue_id) VALUES (3, 'tok', 1)")
	assert.NoError(t, err)

	// running it again is harmless
	assert.NoError(t, CreateModerationTables(x))
}
