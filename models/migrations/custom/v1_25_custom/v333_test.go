// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"testing"

	"code.gitea.io/gitea/models/migrations/base"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_AddIsForkOnlyToUser tests the v333 migration that adds is_fork_only to the user table.
func Test_AddIsForkOnlyToUser(t *testing.T) {
	type User struct {
		ID   int64 `xorm:"pk autoincr"`
		Name string
	}

	x, deferable := base.PrepareTestEnv(t, 0, new(User))
	defer deferable()
	if x == nil || t.Failed() {
		return
	}

	_, err := x.Insert(&User{Name: "existing"})
	require.NoError(t, err)

	require.NoError(t, AddIsForkOnlyToUser(x))

	tables, err := x.DBMetas()
	require.NoError(t, err)
	var found bool
	for _, table := range tables {
		if table.Name == "user" {
			found = table.GetColumn("is_fork_only") != nil
		}
	}
	assert.True(t, found, "user.is_fork_only should exist")

	// existing users are not fork-only
	var isForkOnly bool
	has, err := x.SQL("SELECT is_fork_only FROM `user` WHERE name = ?", "existing").Get(&isForkOnly)
	require.NoError(t, err)
	assert.True(t, has)
	assert.False(t, isForkOnly)

	// running it again is a no-op
	assert.NoError(t, AddIsForkOnlyToUser(x))
}
