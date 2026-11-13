// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// SanctionKind is what a sanction does.
type SanctionKind string

const (
	SanctionWarning    SanctionKind = "warning"    // records only: never touches the user
	SanctionSuspension SanctionKind = "suspension" // Story 3
	SanctionBan        SanctionKind = "ban"        // Story 3
)

// SanctionStatus is where a sanction stands. An expired sanction keeps StatusActive: the
// active queries compare EndsUnix with now, and no cron rewrites rows.
type SanctionStatus string

const (
	SanctionActive          SanctionStatus = "active"
	SanctionPendingApproval SanctionStatus = "pending_approval" // a permanent ban awaiting a second moderator (Story 3)
	SanctionLifted          SanctionStatus = "lifted"           // ended early by an admin (Story 3)
	SanctionOverturned      SanctionStatus = "overturned"       // the decision behind it was reversed
)

// Sanction is one warning, suspension or ban of a user (foundations decisions 7).
type Sanction struct {
	ID                  int64              `xorm:"pk autoincr"`
	UserID              int64              `xorm:"INDEX NOT NULL"`
	Kind                SanctionKind       `xorm:"VARCHAR(16) INDEX NOT NULL"`
	Status              SanctionStatus     `xorm:"VARCHAR(24) INDEX NOT NULL"`
	EndsUnix            timeutil.TimeStamp `xorm:"INDEX NOT NULL DEFAULT 0"` // 0: no end
	CaseIssueID         int64              `xorm:"INDEX NOT NULL DEFAULT 0"` // 0: from the user record
	ConversationIssueID int64              `xorm:"NOT NULL DEFAULT 0"`
	CreatedBy           int64              `xorm:"NOT NULL DEFAULT 0"`
	ApprovedBy          int64              `xorm:"NOT NULL DEFAULT 0"`
	CreatedUnix         timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix         timeutil.TimeStamp `xorm:"updated"`
}

func (*Sanction) TableName() string { return "moderation_sanction" }

func init() {
	db.RegisterModel(new(Sanction))
}

// InsertSanction records a sanction.
func InsertSanction(ctx context.Context, s *Sanction) error {
	return db.Insert(ctx, s)
}

// activeCond matches the sanctions of a kind in force at now.
func activeCond(userID int64, kind SanctionKind, now timeutil.TimeStamp) builder.Cond {
	return builder.Eq{"user_id": userID, "kind": kind, "status": SanctionActive}.
		And(builder.Eq{"ends_unix": 0}.Or(builder.Gt{"ends_unix": now}))
}

// FindActiveWarnings returns the user's warnings in force at now, newest first: the standing
// the owner strip shows.
func FindActiveWarnings(ctx context.Context, userID int64, now timeutil.TimeStamp) ([]*Sanction, error) {
	warnings := make([]*Sanction, 0, 2)
	return warnings, db.GetEngine(ctx).Where(activeCond(userID, SanctionWarning, now)).Desc("id").Find(&warnings)
}

// CountActiveWarnings counts the user's warnings in force at now.
func CountActiveWarnings(ctx context.Context, userID int64, now timeutil.TimeStamp) (int64, error) {
	return db.GetEngine(ctx).Where(activeCond(userID, SanctionWarning, now)).Count(new(Sanction))
}

// HasActiveWarningForCase reports whether the user already has a warning in force from the case.
func HasActiveWarningForCase(ctx context.Context, userID, caseIssueID int64, now timeutil.TimeStamp) (bool, error) {
	return db.GetEngine(ctx).Where(activeCond(userID, SanctionWarning, now).And(builder.Eq{"case_issue_id": caseIssueID})).Exist(new(Sanction))
}
