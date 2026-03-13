// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"xorm.io/xorm"
)

// AddIsForkedToPullRequest adds is_forked and forked_repo_id columns to the
// pull_request table. These track whether a closed Change Request's changes
// were forked by the author into their own repository.
func AddIsForkedToPullRequest(x *xorm.Engine) error {
	type PullRequest struct {
		IsForked     bool  `xorm:"NOT NULL DEFAULT false"`
		ForkedRepoID int64 `xorm:"INDEX"`
	}
	return x.Sync(new(PullRequest))
}
