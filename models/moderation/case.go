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

// Case decisions, stored in Case.Decision; the decision/* label of the same name mirrors it.
const (
	DecisionDismissed        = "dismissed"
	DecisionSourcesRequested = "sources-requested"
	DecisionRemoved          = "removed"
	DecisionEscalated        = "escalated"
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
	ReopenedCount       int                `xorm:"NOT NULL DEFAULT 0"` // incremented by each reopen
	ReopenedUnix        timeutil.TimeStamp // set by each reopen; see IsReexamining
	PreviousCaseIssueID int64              `xorm:"NOT NULL DEFAULT 0"`

	// WaitingOnOwner is set by Request sources (foundations decisions 12): the case stays
	// assigned and counts toward the moderator's limit, but the 48h return skips it until
	// the owner responds (a message in the case conversation, or a change to the article)
	// or the deadline passes. The case is never removed automatically at the deadline.
	WaitingOnOwner bool `xorm:"INDEX NOT NULL DEFAULT false"`

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

// IsReexamining reports whether the case was reopened after its last decision: the earlier
// verdicts stay in place, and reporters are shown that the case is being re-examined, until
// the new decision.
func (c *Case) IsReexamining() bool {
	return c.ReopenedUnix > c.DecidedUnix
}

// SetWaitingOnOwner starts or ends the wait on the owner after Request sources. Ending it
// restarts the 48h timer at now, so the moderator has the usual time to look at the owner's
// response (or at the passed deadline).
func SetWaitingOnOwner(ctx context.Context, caseIssueID int64, waiting bool, now timeutil.TimeStamp) error {
	c := &Case{WaitingOnOwner: waiting}
	cols := []string{"waiting_on_owner"}
	if !waiting {
		c.AssignedUnix = now
		cols = append(cols, "assigned_unix")
	}
	_, err := db.GetEngine(ctx).ID(caseIssueID).Cols(cols...).Update(c)
	return err
}
