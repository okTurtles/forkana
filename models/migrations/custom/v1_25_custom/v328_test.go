// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"strings"
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm/schemas"
)

// Test_AddIsForkedToPullRequest tests the v328 migration that adds
// is_forked and forked_repo_id columns to the pull_request table.
func Test_AddIsForkedToPullRequest(t *testing.T) {
	// Define the PullRequest table structure before migration (minimal columns).
	type PullRequest struct {
		ID         int64  `xorm:"pk autoincr"`
		IssueID    int64  `xorm:"INDEX"`
		HeadRepoID int64  `xorm:"INDEX"`
		BaseRepoID int64  `xorm:"INDEX"`
		HeadBranch string `xorm:"VARCHAR(255)"`
		BaseBranch string `xorm:"VARCHAR(255)"`
		HasMerged  bool   `xorm:"INDEX"`
	}

	// Prepare and load the testing database
	x, deferable := base.PrepareTestEnv(t, 0, new(PullRequest))
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	// Test Case 1: Columns are created with correct properties
	t.Run("ColumnsCreated", func(t *testing.T) {
		err := AddIsForkedToPullRequest(x)
		assert.NoError(t, err)

		tables, err := x.DBMetas()
		assert.NoError(t, err)

		var prTable *schemas.Table
		for _, table := range tables {
			if table.Name == "pull_request" {
				prTable = table
				break
			}
		}
		assert.NotNil(t, prTable, "pull_request table should exist")

		// Verify is_forked column exists
		isForkedCol := prTable.GetColumn("is_forked")
		assert.NotNil(t, isForkedCol, "is_forked column should exist")

		// Verify forked_repo_id column exists
		forkedRepoIDCol := prTable.GetColumn("forked_repo_id")
		assert.NotNil(t, forkedRepoIDCol, "forked_repo_id column should exist")
	})

	// Test Case 2: Default values are applied correctly
	t.Run("DefaultValues", func(t *testing.T) {
		// Insert a row without specifying is_forked or forked_repo_id
		_, err := x.Exec("INSERT INTO pull_request (issue_id, head_repo_id, base_repo_id, head_branch, base_branch, has_merged) VALUES (?, ?, ?, ?, ?, ?)",
			100, 1, 1, "test-branch", "main", false)
		assert.NoError(t, err)

		// Query and verify defaults
		type PRResult struct {
			ID           int64 `xorm:"pk"`
			IsForked     bool
			ForkedRepoID int64
		}
		var results []PRResult
		err = x.Table("pull_request").Where("issue_id = ?", 100).Find(&results)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.False(t, results[0].IsForked, "is_forked should default to false")
		assert.Equal(t, int64(0), results[0].ForkedRepoID, "forked_repo_id should default to 0")

		// Clean up
		_, err = x.Exec("DELETE FROM pull_request WHERE issue_id = ?", 100)
		assert.NoError(t, err)
	})

	// Test Case 3: Index on forked_repo_id exists
	t.Run("IndexExists", func(t *testing.T) {
		tables, err := x.DBMetas()
		assert.NoError(t, err)

		var prTable *schemas.Table
		for _, table := range tables {
			if table.Name == "pull_request" {
				prTable = table
				break
			}
		}
		assert.NotNil(t, prTable)

		// Look for an index containing forked_repo_id
		foundIndex := false
		for _, index := range prTable.Indexes {
			for _, col := range index.Cols {
				cleanCol := strings.Trim(col, `"'`)
				if cleanCol == "forked_repo_id" {
					foundIndex = true
					break
				}
			}
			if foundIndex {
				break
			}
		}
		assert.True(t, foundIndex, "Index on forked_repo_id should exist")
	})

	// Test Case 4: Idempotency — running migration twice should not error
	t.Run("Idempotent", func(t *testing.T) {
		err := AddIsForkedToPullRequest(x)
		assert.NoError(t, err, "Running migration a second time should not error")

		// Verify columns still exist after second run
		tables, err := x.DBMetas()
		assert.NoError(t, err)

		var prTable *schemas.Table
		for _, table := range tables {
			if table.Name == "pull_request" {
				prTable = table
				break
			}
		}
		assert.NotNil(t, prTable)
		assert.NotNil(t, prTable.GetColumn("is_forked"), "is_forked should still exist after second run")
		assert.NotNil(t, prTable.GetColumn("forked_repo_id"), "forked_repo_id should still exist after second run")
	})

	// Test Case 5: Existing rows are not affected by migration
	t.Run("ExistingRowsPreserved", func(t *testing.T) {
		// Insert a row before re-running migration
		_, err := x.Exec("INSERT INTO pull_request (issue_id, head_repo_id, base_repo_id, head_branch, base_branch, has_merged, is_forked, forked_repo_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			200, 2, 2, "feature", "main", false, true, 42)
		assert.NoError(t, err)

		// Re-run migration
		err = AddIsForkedToPullRequest(x)
		assert.NoError(t, err)

		// Verify the existing row is unchanged
		type PRResult struct {
			IsForked     bool
			ForkedRepoID int64
		}
		var results []PRResult
		err = x.Table("pull_request").Where("issue_id = ?", 200).Find(&results)
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.True(t, results[0].IsForked, "is_forked should remain true")
		assert.Equal(t, int64(42), results[0].ForkedRepoID, "forked_repo_id should remain 42")
	})
}
