// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/xorm"
)

// CreateModerationSanctionTable creates moderation_sanction: the warnings, suspensions and
// bans of a user, what their standing is made of. Story 1 only writes warnings.
func CreateModerationSanctionTable(x *xorm.Engine) error {
	type ModerationSanction struct {
		ID                  int64              `xorm:"pk autoincr"`
		UserID              int64              `xorm:"INDEX NOT NULL"`
		Kind                string             `xorm:"VARCHAR(16) INDEX NOT NULL"`
		Status              string             `xorm:"VARCHAR(24) INDEX NOT NULL"`
		EndsUnix            timeutil.TimeStamp `xorm:"INDEX NOT NULL DEFAULT 0"`
		CaseIssueID         int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
		ConversationIssueID int64              `xorm:"NOT NULL DEFAULT 0"`
		CreatedBy           int64              `xorm:"NOT NULL DEFAULT 0"`
		ApprovedBy          int64              `xorm:"NOT NULL DEFAULT 0"`
		CreatedUnix         timeutil.TimeStamp `xorm:"INDEX created"`
		UpdatedUnix         timeutil.TimeStamp `xorm:"updated"`
	}
	return x.Sync(new(ModerationSanction))
}
