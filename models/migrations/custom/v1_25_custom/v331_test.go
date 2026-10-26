// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
)

// Test_CreateArticleCollaboratorTable tests the v331 migration that creates the
// article_collaborator table.
func Test_CreateArticleCollaboratorTable(t *testing.T) {
	x, deferable := base.PrepareTestEnv(t, 0)
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	assert.NoError(t, CreateArticleCollaboratorTable(x))

	tables, err := x.DBMetas()
	assert.NoError(t, err)
	var found bool
	for _, table := range tables {
		if table.Name != "article_collaborator" {
			continue
		}
		found = true
		for _, col := range []string{"id", "repo_id", "user_id", "inviter_id", "role", "status", "created_unix", "updated_unix"} {
			assert.NotNil(t, table.GetColumn(col), "%s column should exist", col)
		}
	}
	assert.True(t, found, "article_collaborator table should exist")

	// a repository cannot list the same user twice
	_, err = x.Exec("INSERT INTO article_collaborator (repo_id, user_id, inviter_id, role, status) VALUES (1, 2, 1, 1, 0)")
	assert.NoError(t, err)
	_, err = x.Exec("INSERT INTO article_collaborator (repo_id, user_id, inviter_id, role, status) VALUES (1, 2, 1, 2, 0)")
	assert.Error(t, err)

	// running it again is a no-op
	assert.NoError(t, CreateArticleCollaboratorTable(x))
}
