// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// CaseKind is what a case is about. It is authoritative; the kind/* label mirrors it.
type CaseKind string

const (
	CaseKindArticle CaseKind = "article"
	CaseKindFork    CaseKind = "fork"
	CaseKindUser    CaseKind = "user"
	CaseKindSubject CaseKind = "subject"
)

// LabelName returns the name of the kind's label.
func (k CaseKind) LabelName() string {
	return "kind/" + string(k)
}

// Case is the moderation data of a case issue: all the reports on one target. Its fields are
// authoritative; the case issue's labels are projections of them (foundations decisions 1.2).
type Case struct {
	IssueID         int64    `xorm:"pk"`
	Kind            CaseKind `xorm:"VARCHAR(16) INDEX NOT NULL"`
	TargetRepoID    int64    `xorm:"INDEX NOT NULL DEFAULT 0"`
	TargetSubjectID int64    `xorm:"INDEX NOT NULL DEFAULT 0"`
	TargetUserID    int64    `xorm:"INDEX NOT NULL DEFAULT 0"`
	FlagDetail      string   `xorm:"VARCHAR(255)"` // Story 2
	FlaggedBlocks   string   `xorm:"TEXT"`         // JSON, written by the Request sources action

	// Decision, Resolution and NeedsAdmin are mirrored by the decision/*, resolution/* and
	// status/needs-admin labels.
	Decision   string `xorm:"VARCHAR(32) INDEX NOT NULL DEFAULT ''"`
	Resolution string `xorm:"VARCHAR(32) INDEX NOT NULL DEFAULT ''"`
	NeedsAdmin bool   `xorm:"INDEX NOT NULL DEFAULT false"`

	AssignedUnix        timeutil.TimeStamp // the 48h timer start
	TakenUnix           timeutil.TimeStamp
	DecidedBy           int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
	DecidedUnix         timeutil.TimeStamp `xorm:"INDEX"`
	ReopenedCount       int                `xorm:"NOT NULL DEFAULT 0"`
	PreviousCaseIssueID int64              `xorm:"NOT NULL DEFAULT 0"`

	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
}

// TableName keeps the table name explicit: the type is moderation.Case.
func (*Case) TableName() string { return "moderation_case" }

func init() {
	db.RegisterModel(new(Case))
	db.RegisterModel(new(Submission))
	db.RegisterModel(new(Report))
	db.RegisterModel(new(Audit))
}

// InsertCase records the moderation data of a new case issue.
func InsertCase(ctx context.Context, c *Case) error {
	return db.Insert(ctx, c)
}

// GetCase returns the case of a case issue, or nil if the issue isn't one.
func GetCase(ctx context.Context, issueID int64) (*Case, error) {
	c := new(Case)
	has, err := db.GetEngine(ctx).ID(issueID).Get(c)
	if err != nil || !has {
		return nil, err
	}
	return c, nil
}

// FindOpenCaseByTargetRepo returns the open case of a repository target, or nil. Callers that
// may create one hold the target's global lock and call it inside their transaction.
func FindOpenCaseByTargetRepo(ctx context.Context, repoID int64) (*Case, error) {
	c := new(Case)
	has, err := db.GetEngine(ctx).
		Join("INNER", "issue", "issue.id = moderation_case.issue_id").
		Where(builder.Eq{"moderation_case.target_repo_id": repoID, "issue.is_closed": false}).
		Desc("moderation_case.issue_id").
		Get(c)
	if err != nil || !has {
		return nil, err
	}
	return c, nil
}
