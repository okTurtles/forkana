// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/xorm"
)

// CreateModerationTables creates the moderation tables of Story 1: cases, report submissions,
// the per-reason reports, and the append-only audit log. Reports and cases are issues of the
// private cases repository; these tables hold what issues can't. Some columns are only filled
// by later stories, so that they need no migration of their own.
func CreateModerationTables(x *xorm.Engine) error {
	// ModerationCase is the moderation data of a case issue (all the reports on one target).
	type ModerationCase struct {
		IssueID         int64  `xorm:"pk"`
		Kind            string `xorm:"VARCHAR(16) INDEX NOT NULL"`
		TargetRepoID    int64  `xorm:"INDEX NOT NULL DEFAULT 0"`
		TargetSubjectID int64  `xorm:"INDEX NOT NULL DEFAULT 0"`
		TargetUserID    int64  `xorm:"INDEX NOT NULL DEFAULT 0"`
		FlagDetail      string `xorm:"VARCHAR(255)"`
		FlaggedBlocks   string `xorm:"TEXT"`

		Decision   string `xorm:"VARCHAR(32) INDEX NOT NULL DEFAULT ''"`
		Resolution string `xorm:"VARCHAR(32) INDEX NOT NULL DEFAULT ''"`
		NeedsAdmin bool   `xorm:"INDEX NOT NULL DEFAULT false"`

		AssignedUnix        timeutil.TimeStamp
		TakenUnix           timeutil.TimeStamp
		DecidedBy           int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
		DecidedUnix         timeutil.TimeStamp `xorm:"INDEX"`
		ReopenedCount       int                `xorm:"NOT NULL DEFAULT 0"`
		ReopenedUnix        timeutil.TimeStamp
		PreviousCaseIssueID int64 `xorm:"NOT NULL DEFAULT 0"`
		WaitingOnOwner      bool  `xorm:"INDEX NOT NULL DEFAULT false"`

		CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
		UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
	}

	// ModerationSubmission is one send of the report form: one reporter, one anchor.
	type ModerationSubmission struct {
		ID          int64  `xorm:"pk autoincr"`
		ReporterID  int64  `xorm:"UNIQUE(reporter_client_token) INDEX NOT NULL"`
		ClientToken string `xorm:"UNIQUE(reporter_client_token) VARCHAR(36) NOT NULL"`
		CaseIssueID int64  `xorm:"INDEX NOT NULL"`

		CommitSHA     string `xorm:"VARCHAR(64) NOT NULL DEFAULT ''"`
		TreePath      string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		SourceStart   int64  `xorm:"NOT NULL DEFAULT 0"`
		SourceEnd     int64  `xorm:"NOT NULL DEFAULT 0"`
		Quote         string `xorm:"TEXT"`
		SectionAnchor string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		AnchorStatus  string `xorm:"VARCHAR(16) NOT NULL DEFAULT 'whole'"`

		CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	}

	// ModerationReport is the moderation data of a reason issue: one reason of one submission.
	type ModerationReport struct {
		IssueID      int64  `xorm:"pk"`
		SubmissionID int64  `xorm:"INDEX NOT NULL"`
		CaseIssueID  int64  `xorm:"INDEX NOT NULL"`
		ReporterID   int64  `xorm:"INDEX NOT NULL"`
		Reason       string `xorm:"VARCHAR(32) INDEX NOT NULL"`

		Verdict     string `xorm:"VARCHAR(16) INDEX NOT NULL DEFAULT 'pending'"`
		VerdictNote string `xorm:"TEXT"`
		VerdictBy   int64  `xorm:"NOT NULL DEFAULT 0"`
		VerdictUnix timeutil.TimeStamp
		OnHold      bool `xorm:"INDEX NOT NULL DEFAULT false"`

		CreatedUnix timeutil.TimeStamp `xorm:"created"`
		UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
	}

	// ModerationAudit is one entry of the moderation audit log. Append-only.
	type ModerationAudit struct {
		ID                  int64              `xorm:"pk autoincr"`
		CreatedUnix         timeutil.TimeStamp `xorm:"INDEX created"`
		ActorID             int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
		ActorRole           string             `xorm:"VARCHAR(16) NOT NULL"`
		Action              string             `xorm:"VARCHAR(32) INDEX NOT NULL"`
		CaseIssueID         int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
		ConversationIssueID int64              `xorm:"NOT NULL DEFAULT 0"`
		TargetUserID        int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
		TargetRepoID        int64              `xorm:"NOT NULL DEFAULT 0"`
		Reason              string             `xorm:"VARCHAR(32) NOT NULL DEFAULT ''"`
		Summary             string             `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
		CommentID           int64              `xorm:"NOT NULL DEFAULT 0"`
	}

	return x.Sync(new(ModerationCase), new(ModerationSubmission), new(ModerationReport), new(ModerationAudit))
}
