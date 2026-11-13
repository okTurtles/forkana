// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_CreateModerationSanctionTable tests the v336 migration that creates moderation_sanction.
func Test_CreateModerationSanctionTable(t *testing.T) {
	x, deferable := base.PrepareTestEnv(t, 0)
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	require.NoError(t, CreateModerationSanctionTable(x))
	tables, err := x.DBMetas()
	require.NoError(t, err)
	found := false
	for _, table := range tables {
		if table.Name != "moderation_sanction" {
			continue
		}
		found = true
		for _, col := range []string{"id", "user_id", "kind", "status", "ends_unix", "case_issue_id", "conversation_issue_id", "created_by", "approved_by", "created_unix", "updated_unix"} {
			assert.NotNil(t, table.GetColumn(col), "moderation_sanction.%s should exist", col)
		}
	}
	assert.True(t, found, "moderation_sanction should exist")

	// running it again is harmless
	assert.NoError(t, CreateModerationSanctionTable(x))
}
