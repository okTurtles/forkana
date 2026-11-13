// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// ActorRole is the role an audited action was taken in, at the time it was taken.
type ActorRole string

const (
	ActorUser      ActorRole = "user"      // a reader filing a report
	ActorModerator ActorRole = "moderator" // a member of the moderators team
	ActorAdmin     ActorRole = "admin"     // a site administrator, whether or not also in the team
	ActorAutomatic ActorRole = "automatic" // Forkana itself; ActorID is 0
)

// AuditAction is an entry of the audit catalogue (foundations decisions 6). Later stories
// append actions; existing ones are never renamed.
type AuditAction string

const (
	AuditReportFiled     AuditAction = "report_filed"
	AuditAssigned        AuditAction = "assigned"
	AuditTaken           AuditAction = "taken"
	AuditReturnedToQueue AuditAction = "returned_to_queue"
	AuditDeadlinePassed  AuditAction = "deadline_passed" // automatic: the sources deadline passed; nothing is removed
	AuditDecided         AuditAction = "decided"
	AuditWarned          AuditAction = "warned"
	AuditReopened        AuditAction = "reopened"
	AuditEscalated       AuditAction = "escalated"
)

// Audit is one entry of the moderation audit log. It is append-only: this package inserts and
// reads entries, and has nothing to change or delete one. Entries are written by
// services/moderation in the same transaction as the action they record. Summary is made by
// the service from a fixed template: never text a person typed, an IP or an email address.
type Audit struct {
	ID                  int64              `xorm:"pk autoincr"`
	CreatedUnix         timeutil.TimeStamp `xorm:"INDEX created"`
	ActorID             int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
	ActorRole           ActorRole          `xorm:"VARCHAR(16) NOT NULL"`
	Action              AuditAction        `xorm:"VARCHAR(32) INDEX NOT NULL"`
	CaseIssueID         int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
	ConversationIssueID int64              `xorm:"NOT NULL DEFAULT 0"`
	TargetUserID        int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
	TargetRepoID        int64              `xorm:"NOT NULL DEFAULT 0"`
	Reason              Reason             `xorm:"VARCHAR(32) NOT NULL DEFAULT ''"`
	Summary             string             `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
	CommentID           int64              `xorm:"NOT NULL DEFAULT 0"`
}

func (*Audit) TableName() string { return "moderation_audit" }

// InsertAudit appends an entry to the audit log.
func InsertAudit(ctx context.Context, a *Audit) error {
	return db.Insert(ctx, a)
}

// FindAuditByCase returns a case's audit entries, oldest first.
func FindAuditByCase(ctx context.Context, caseIssueID int64) ([]*Audit, error) {
	entries := make([]*Audit, 0, 4)
	return entries, db.GetEngine(ctx).Where(builder.Eq{"case_issue_id": caseIssueID}).Asc("id").Find(&entries)
}
