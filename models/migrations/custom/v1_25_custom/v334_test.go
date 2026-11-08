// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
)

// Test_CreateSubjectFollowTable tests the v334 migration that creates the subject_follow
// table.
func Test_CreateSubjectFollowTable(t *testing.T) {
	x, deferable := base.PrepareTestEnv(t, 0)
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	assert.NoError(t, CreateSubjectFollowTable(x))

	tables, err := x.DBMetas()
	assert.NoError(t, err)
	var found bool
	for _, table := range tables {
		if table.Name != "subject_follow" {
			continue
		}
		found = true
		for _, col := range []string{"id", "user_id", "subject_id", "created_unix"} {
			assert.NotNil(t, table.GetColumn(col), "%s column should exist", col)
		}
	}
	assert.True(t, found, "subject_follow table should exist")

	// a user cannot follow the same subject twice
	_, err = x.Exec("INSERT INTO subject_follow (user_id, subject_id) VALUES (2, 1)")
	assert.NoError(t, err)
	_, err = x.Exec("INSERT INTO subject_follow (user_id, subject_id) VALUES (2, 1)")
	assert.Error(t, err)
	_, err = x.Exec("INSERT INTO subject_follow (user_id, subject_id) VALUES (3, 1)")
	assert.NoError(t, err)

	// running it again is harmless
	assert.NoError(t, CreateSubjectFollowTable(x))
}
