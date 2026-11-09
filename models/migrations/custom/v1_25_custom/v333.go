// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"xorm.io/xorm"
)

// AddIsForkOnlyToUser adds the is_fork_only column to the user table: a fork-only user
// (like an account importing Wikipedia articles) does not accept change requests, so
// editing their articles creates a fork instead.
func AddIsForkOnlyToUser(x *xorm.Engine) error {
	type User struct {
		IsForkOnly bool `xorm:"NOT NULL DEFAULT false"`
	}

	return x.Sync(new(User))
}
