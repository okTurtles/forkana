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

// Who a case waits for (Case.WaitingOn).
const (
	WaitingOnOwner = "owner" // sources requested: the owner replies, or adds a source
	WaitingOnAdmin = "admin" // escalated: a site admin decides
)

// Why a case came back to its moderator (Case.Attention).
const (
	AttentionNeedsReply     = "needs_reply"     // the owner replied or added a source; the moderator decides if it's enough
	AttentionDeadlinePassed = "deadline_passed" // the sources deadline passed; nothing is removed automatically
	AttentionAdminDecided   = "admin_decided"   // the site admin decided an escalated case
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
	ReopenedUnix        timeutil.TimeStamp // set by each reopen; the case shows "Reopened", see IsReexamining
	PreviousCaseIssueID int64              `xorm:"NOT NULL DEFAULT 0"`

	// WaitingOn is who the case waits for (foundations decisions 12): the owner after Request
	// sources, a site admin after Escalate. The case keeps its moderator and counts toward
	// their limit, but the 48h return skips it while it waits.
	WaitingOn string `xorm:"VARCHAR(16) INDEX NOT NULL DEFAULT ''"`
	// Attention is why the case came back to its moderator (ReturnToModerator), shown on the
	// case until the moderator acts on it.
	Attention string `xorm:"VARCHAR(24) INDEX NOT NULL DEFAULT ''"`

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

// StartWaiting marks the case as waiting for the owner or a site admin. The 48h return skips
// it until ReturnToModerator.
func StartWaiting(ctx context.Context, caseIssueID int64, on string) error {
	_, err := db.GetEngine(ctx).ID(caseIssueID).Cols("waiting_on", "attention").Update(&Case{WaitingOn: on})
	return err
}

// ReturnToModerator ends the wait and gives the case back to its moderator with why it came
// back. The 48h timer restarts at now. A case nobody holds stays in the Queue, where the
// deadline-passed priority puts it on top.
func ReturnToModerator(ctx context.Context, caseIssueID int64, attention string, now timeutil.TimeStamp) error {
	_, err := db.GetEngine(ctx).ID(caseIssueID).Cols("waiting_on", "attention", "assigned_unix").
		Update(&Case{Attention: attention, AssignedUnix: now})
	return err
}

// ClearAttention is called by the moderator's next action on the case.
func ClearAttention(ctx context.Context, caseIssueID int64) error {
	_, err := db.GetEngine(ctx).ID(caseIssueID).Cols("attention").Update(&Case{})
	return err
}
