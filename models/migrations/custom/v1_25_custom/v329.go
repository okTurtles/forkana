// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/xorm"
)

// AddTombstoneToRepository adds is_tombstoned and tombstoned_unix columns to the
// repository table. A tombstoned repository was deleted by its author while forks
// still existed, so its row and git data are retained for fork ancestry.
func AddTombstoneToRepository(x *xorm.Engine) error {
	type Repository struct {
		IsTombstoned   bool               `xorm:"INDEX NOT NULL DEFAULT false"`
		TombstonedUnix timeutil.TimeStamp `xorm:"DEFAULT 0"`
	}
	return x.Sync(new(Repository))
}
