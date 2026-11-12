// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/xorm"
)

// CreateSubjectFollowTable creates the subject_follow table, which keeps the subjects each
// user follows (#165): their activity, on every article of the subject, is in the user's feed.
func CreateSubjectFollowTable(x *xorm.Engine) error {
	type SubjectFollow struct {
		ID          int64              `xorm:"pk autoincr"`
		UserID      int64              `xorm:"UNIQUE(subject_follow) NOT NULL"`
		SubjectID   int64              `xorm:"UNIQUE(subject_follow) INDEX NOT NULL"`
		CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	}

	return x.Sync(new(SubjectFollow))
}
