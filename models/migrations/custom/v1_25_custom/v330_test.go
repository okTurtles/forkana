// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"strings"
	"testing"

	"code.gitea.io/gitea/models/migrations/base"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm/schemas"
)

// Test_AddArticleAttachmentTable tests the v330 migration that creates the
// article_attachment table and adds attachment.purpose.
func Test_AddArticleAttachmentTable(t *testing.T) {
	// Attachment table structure before migration (minimal columns).
	type Attachment struct {
		ID          int64  `xorm:"pk autoincr"`
		UUID        string `xorm:"uuid UNIQUE"`
		RepoID      int64  `xorm:"INDEX"`
		IssueID     int64  `xorm:"INDEX"`
		ReleaseID   int64  `xorm:"INDEX"`
		CommentID   int64  `xorm:"INDEX"`
		Name        string
		CreatedUnix timeutil.TimeStamp `xorm:"created"`
	}

	x, deferable := base.PrepareTestEnv(t, 0, new(Attachment))
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	findTable := func(t *testing.T, name string) *schemas.Table {
		t.Helper()
		tables, err := x.DBMetas()
		assert.NoError(t, err)
		for _, table := range tables {
			if table.Name == name {
				return table
			}
		}
		return nil
	}

	// Test Case 1: the table and its columns are created
	t.Run("TableCreated", func(t *testing.T) {
		assert.NoError(t, AddArticleAttachmentTable(x))

		table := findTable(t, "article_attachment")
		assert.NotNil(t, table, "article_attachment table should exist")
		assert.NotNil(t, table.GetColumn("id"), "id column should exist")
		assert.NotNil(t, table.GetColumn("repo_id"), "repo_id column should exist")
		assert.NotNil(t, table.GetColumn("attachment_id"), "attachment_id column should exist")
		assert.NotNil(t, table.GetColumn("created_unix"), "created_unix column should exist")
	})

	// Test Case 2: the purpose discriminator is added to attachment
	t.Run("PurposeColumnAdded", func(t *testing.T) {
		table := findTable(t, "attachment")
		assert.NotNil(t, table, "attachment table should exist")
		assert.NotNil(t, table.GetColumn("purpose"), "purpose column should exist")

		// The xorm "uuid" tag maps to PostgreSQL's native uuid type, which rejects any
		// literal that is not a well-formed UUID, so this cannot be a readable label.
		const legacyUUID = "3f1b9c2e-7d4a-4c8b-9e5f-0a1b2c3d4e5f"

		_, err := x.Exec("INSERT INTO attachment (uuid, repo_id, issue_id, release_id, comment_id, name) VALUES (?, ?, ?, ?, ?, ?)",
			legacyUUID, 1, 0, 0, 0, "attach")
		assert.NoError(t, err)

		type attachmentResult struct {
			Purpose int
		}
		var results []attachmentResult
		assert.NoError(t, x.Table("attachment").Where("uuid = ?", legacyUUID).Find(&results))
		assert.Len(t, results, 1)
		assert.Equal(t, 0, results[0].Purpose, "purpose should default to 0 for legacy rows")
	})

	// Test Case 3: both required indexes exist, and the composite one is unique
	t.Run("IndexesCreated", func(t *testing.T) {
		table := findTable(t, "article_attachment")
		assert.NotNil(t, table)

		cleanCols := func(cols []string) []string {
			out := make([]string, 0, len(cols))
			for _, col := range cols {
				out = append(out, strings.Trim(col, `"'`))
			}
			return out
		}

		var uniqueComposite, repoIndex, attachmentIndex bool
		for _, index := range table.Indexes {
			cols := cleanCols(index.Cols)
			if len(cols) == 2 && cols[0] == "repo_id" && cols[1] == "attachment_id" && index.Type == schemas.UniqueType {
				uniqueComposite = true
			}
			if cols[0] == "repo_id" {
				repoIndex = true
			}
			if cols[0] == "attachment_id" {
				attachmentIndex = true
			}
		}
		assert.True(t, uniqueComposite, "unique index on (repo_id, attachment_id) should exist")
		assert.True(t, repoIndex, "an index starting with repo_id should exist")
		assert.True(t, attachmentIndex, "an index starting with attachment_id should exist")
	})

	// Test Case 4: the unique constraint is enforced
	t.Run("UniqueConstraintEnforced", func(t *testing.T) {
		_, err := x.Exec("INSERT INTO article_attachment (repo_id, attachment_id, created_unix) VALUES (?, ?, ?)", 1, 1, 946684800)
		assert.NoError(t, err)

		_, err = x.Exec("INSERT INTO article_attachment (repo_id, attachment_id, created_unix) VALUES (?, ?, ?)", 1, 1, 946684800)
		assert.Error(t, err, "duplicate (repo_id, attachment_id) should be rejected")

		// A different repository may reference the same attachment.
		_, err = x.Exec("INSERT INTO article_attachment (repo_id, attachment_id, created_unix) VALUES (?, ?, ?)", 2, 1, 946684800)
		assert.NoError(t, err)
	})

	// Test Case 5: idempotency — running the migration twice keeps the data
	t.Run("Idempotent", func(t *testing.T) {
		assert.NoError(t, AddArticleAttachmentTable(x), "running migration a second time should not error")

		count, err := x.Table("article_attachment").Where("attachment_id = ?", 1).Count()
		assert.NoError(t, err)
		assert.EqualValues(t, 2, count, "existing associations should be preserved")

		assert.NotNil(t, findTable(t, "article_attachment").GetColumn("attachment_id"))
		assert.NotNil(t, findTable(t, "attachment").GetColumn("purpose"))
	})
}
