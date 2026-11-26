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
	DecisionResolved         = "resolved" // the owner fixed it: the reports are upheld, nothing goes on the owner's record
	DecisionSourcesRequested = "sources-requested"
	DecisionRemoved          = "removed" // Case.RemoveScope says what was removed
	DecisionEscalated        = "escalated"
)

// What a Remove decision removed (Case.RemoveScope).
const (
	RemoveSection = "section" // the case's section
	RemoveArticle = "article" // the whole article: the article's other open cases close with ResolutionArticleRemoved
)

// How a case ended without a decision of its own (Case.Resolution). The other resolutions
// (sources added, removed after the deadline, overturned) follow a decision.
const (
	ResolutionMerged         = "merged"          // Change part moved it onto an open case with the same key: Case.MergedIntoIssueID
	ResolutionArticleRemoved = "article-removed" // another case on the article removed the whole article
	ResolutionOverturned     = "overturned"      // the case's decision was reversed
)

// SectionWholeArticle and SectionIntro are the two Case.SectionAnchor values that aren't a
// heading's id: a report on the whole article, and a passage before the first heading. Heading
// ids are slugs, which never contain a colon.
const (
	SectionWholeArticle = ""
	SectionIntro        = ":intro"
)

// Who a case waits for (Case.WaitingOn).
const (
	WaitingOnOwner = "owner" // sources requested: the owner replies, or adds a source
	WaitingOnAdmin = "admin" // escalated: a site admin decides
)

// Why a case came back to its moderator (Case.Attention).
const (
	AttentionNeedsReply     = "needs_reply"     // the owner replied; the moderator answers
	AttentionSourceAdded    = "source_added"    // the owner added a source; the moderator decides if it's enough
	AttentionDeadlinePassed = "deadline_passed" // the sources deadline passed; nothing is removed automatically
	AttentionAdminDecided   = "admin_decided"   // the site admin decided an escalated case
)

// LabelName returns the name of the kind's label.
func (k CaseKind) LabelName() string {
	return "kind/" + string(k)
}

// Case is the moderation data of a case issue: all the reports for one reason on one part of
// one target (foundations decisions 13). An article can have several open cases, one per
// section and reason. Its fields are authoritative; the case issue's labels are projections of
// them (foundations decisions 1.2).
// MaxSectionLength is the length, in characters, of the section_anchor and section_title
// columns of moderation_case and moderation_submission (VARCHAR(255)).
const MaxSectionLength = 255

type Case struct {
	IssueID      int64    `xorm:"pk"`
	Kind         CaseKind `xorm:"VARCHAR(16) INDEX NOT NULL"`
	TargetRepoID int64    `xorm:"INDEX(target_part) NOT NULL DEFAULT 0"` // the composite index also serves lookups by article
	// SectionAnchor is the reported part: a heading's id, SectionIntro or SectionWholeArticle.
	// SectionTitle is that heading's text when the case was opened ("" for the other two).
	SectionAnchor   string `xorm:"VARCHAR(255) INDEX(target_part) NOT NULL DEFAULT ''"`
	SectionTitle    string `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
	Reason          Reason `xorm:"VARCHAR(32) INDEX(target_part) NOT NULL DEFAULT ''"`
	TargetSubjectID int64  `xorm:"INDEX NOT NULL DEFAULT 0"`
	TargetUserID    int64  `xorm:"INDEX NOT NULL DEFAULT 0"`
	FlagDetail      string `xorm:"VARCHAR(255)"` // Story 2
	FlaggedBlocks   string `xorm:"TEXT"`         // JSON, written by the Request sources action

	// Decision, Resolution and NeedsAdmin are mirrored by the decision/*, resolution/* and
	// status/needs-admin labels.
	Decision   string `xorm:"VARCHAR(32) INDEX NOT NULL DEFAULT ''"`
	Resolution string `xorm:"VARCHAR(32) INDEX NOT NULL DEFAULT ''"`
	NeedsAdmin bool   `xorm:"INDEX NOT NULL DEFAULT false"`
	// RemoveScope is RemoveSection or RemoveArticle when Decision is DecisionRemoved.
	RemoveScope string `xorm:"VARCHAR(16) NOT NULL DEFAULT ''"`
	// ReporterNote is what the decision tells the case's reporters; required for a dismissal.
	// One note per case: every report in it is for the same reason on the same part.
	ReporterNote string `xorm:"TEXT"`
	// MergedIntoIssueID is the case this one was merged into (ResolutionMerged).
	MergedIntoIssueID int64 `xorm:"NOT NULL DEFAULT 0"`

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

// openCasesOfRepo selects the open cases of a repository target; the query joins issue.
func openCasesOfRepo(repoID int64) builder.Cond {
	return builder.Eq{"moderation_case.target_repo_id": repoID, "issue.is_closed": false}
}

// FindOpenCase returns the open case for a reason on a part of a repository target, or nil.
// Callers that may create one hold the target's global lock and call it inside their
// transaction.
func FindOpenCase(ctx context.Context, repoID int64, section string, reason Reason) (*Case, error) {
	c := new(Case)
	has, err := db.GetEngine(ctx).
		Join("INNER", "issue", "issue.id = moderation_case.issue_id").
		Where(openCasesOfRepo(repoID).And(builder.Eq{"moderation_case.section_anchor": section, "moderation_case.reason": reason})).
		Desc("moderation_case.issue_id").
		Get(c)
	if err != nil || !has {
		return nil, err
	}
	return c, nil
}

// FindOpenCasesByTargetRepo returns every open case of a repository target, oldest first: the
// case page's switcher, and the cases a whole-article removal closes.
func FindOpenCasesByTargetRepo(ctx context.Context, repoID int64) ([]*Case, error) {
	cases := make([]*Case, 0, 2)
	return cases, db.GetEngine(ctx).
		Join("INNER", "issue", "issue.id = moderation_case.issue_id").
		Where(openCasesOfRepo(repoID)).
		Asc("moderation_case.issue_id").Find(&cases)
}

// HasPriorRemoval reports whether a closed case on the repository removed a section or the
// whole article: the case page's "Removed before" marker. A removal that was overturned
// doesn't count.
func HasPriorRemoval(ctx context.Context, repoID int64) (bool, error) {
	return db.GetEngine(ctx).Table("moderation_case").
		Join("INNER", "issue", "issue.id = moderation_case.issue_id").
		Where(builder.Eq{
			"moderation_case.target_repo_id": repoID,
			"moderation_case.decision":       DecisionRemoved,
			"issue.is_closed":                true,
		}.And(builder.Neq{"moderation_case.resolution": ResolutionOverturned})).
		Exist()
}

// SetCasePart moves a case to another part of its target (Change part). The caller holds the
// target's lock and has checked that no other open case has the new key.
func SetCasePart(ctx context.Context, caseIssueID int64, section, title string) error {
	_, err := db.GetEngine(ctx).ID(caseIssueID).Cols("section_anchor", "section_title").
		Update(&Case{SectionAnchor: section, SectionTitle: title})
	return err
}

// MarkMerged records that a case was merged into another.
func MarkMerged(ctx context.Context, caseIssueID, intoIssueID int64) error {
	_, err := db.GetEngine(ctx).ID(caseIssueID).Cols("resolution", "merged_into_issue_id").
		Update(&Case{Resolution: ResolutionMerged, MergedIntoIssueID: intoIssueID})
	return err
}

// IsReexamining reports whether the case was reopened after its last decision: the earlier
// verdicts stay in place, and reporters are shown that the case is being re-examined, until
// the new decision.
func (c *Case) IsReexamining() bool {
	return c.ReopenedUnix > c.DecidedUnix
}

// StartWaiting marks the case as waiting for the owner or a site admin. The 48h return skips
// it until ReturnToModerator. It clears Attention: the moderator has acted on it.
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
