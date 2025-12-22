// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"slices"
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm/schemas"
)

// Test_AddCompositeIndexesForForkOnEdit tests the v327 migration that adds
// composite indexes to optimize fork-on-edit permission queries.
func Test_AddCompositeIndexesForForkOnEdit(t *testing.T) {
	// Define the Repository table structure for testing
	// Only include the columns relevant to the indexes being created
	type Repository struct {
		ID        int64 `xorm:"pk autoincr"`
		OwnerID   int64 `xorm:"INDEX"`
		SubjectID int64 `xorm:"INDEX"`
		ForkID    int64 `xorm:"INDEX"`
	}

	// Prepare and load the testing database
	x, deferable := base.PrepareTestEnv(t, 0, new(Repository))
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	// Helper function to check if an index with specific columns exists
	hasIndexWithColumns := func(table *schemas.Table, cols []string) bool {
		for _, index := range table.Indexes {
			if len(index.Cols) == len(cols) {
				match := true
				for _, col := range cols {
					found := slices.Contains(index.Cols, col)
					if !found {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
		return false
	}

	// Test Case 1: Verify indexes are created correctly
	t.Run("IndexesCreated", func(t *testing.T) {
		// Run the migration
		err := AddCompositeIndexesForForkOnEdit(x)
		assert.NoError(t, err)

		// Get table metadata
		tables, err := x.DBMetas()
		assert.NoError(t, err)

		// Find the repository table
		var repoTable *schemas.Table
		for _, table := range tables {
			if table.Name == "repository" {
				repoTable = table
				break
			}
		}
		assert.NotNil(t, repoTable, "Repository table should exist")

		// Check for composite index on (owner_id, subject_id)
		assert.True(t, hasIndexWithColumns(repoTable, []string{"owner_id", "subject_id"}),
			"Composite index on (owner_id, subject_id) should exist")

		// Check for composite index on (owner_id, fork_id)
		assert.True(t, hasIndexWithColumns(repoTable, []string{"owner_id", "fork_id"}),
			"Composite index on (owner_id, fork_id) should exist")
	})

	// Test Case 2: Verify migration is idempotent (can run multiple times)
	t.Run("Idempotent", func(t *testing.T) {
		// Run the migration again (should not error)
		err := AddCompositeIndexesForForkOnEdit(x)
		assert.NoError(t, err, "Migration should be idempotent and not error on second run")

		// Verify indexes still exist
		tables, err := x.DBMetas()
		assert.NoError(t, err)

		var repoTable *schemas.Table
		for _, table := range tables {
			if table.Name == "repository" {
				repoTable = table
				break
			}
		}
		assert.NotNil(t, repoTable)

		// Verify the composite indexes still exist after running twice
		assert.True(t, hasIndexWithColumns(repoTable, []string{"owner_id", "subject_id"}),
			"Composite index on (owner_id, subject_id) should still exist after second run")
		assert.True(t, hasIndexWithColumns(repoTable, []string{"owner_id", "fork_id"}),
			"Composite index on (owner_id, fork_id) should still exist after second run")
	})

	// Test Case 3: Verify indexes are regular (non-unique) indexes
	t.Run("IndexType", func(t *testing.T) {
		tables, err := x.DBMetas()
		assert.NoError(t, err)

		var repoTable *schemas.Table
		for _, table := range tables {
			if table.Name == "repository" {
				repoTable = table
				break
			}
		}
		assert.NotNil(t, repoTable)

		// Find the composite indexes and verify they are regular indexes
		for _, index := range repoTable.Indexes {
			if len(index.Cols) == 2 {
				isOwnerSubject := hasIndexWithColumns(repoTable, []string{"owner_id", "subject_id"})
				isOwnerFork := hasIndexWithColumns(repoTable, []string{"owner_id", "fork_id"})
				if isOwnerSubject || isOwnerFork {
					// These should be regular indexes, not unique indexes
					assert.Equal(t, schemas.IndexType, index.Type,
						"Composite index should be a regular index, not unique")
				}
			}
		}
	})
}
