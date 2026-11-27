// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// Verdict is the moderator's verdict on one reason. The outcome/* label mirrors it.
type Verdict string

const (
	VerdictPending   Verdict = "pending"
	VerdictUpheld    Verdict = "upheld"
	VerdictDismissed Verdict = "dismissed"
	// VerdictClosed: the case closed without a decision on it, because another case removed
	// the whole article (ResolutionArticleRemoved). The reporter is told the article was removed.
	VerdictClosed Verdict = "closed"
)

// Report is the moderation data of a reason issue: one reason of one submission. The text the
// reporter is told about the verdict is the case's ReporterNote.
type Report struct {
	IssueID      int64  `xorm:"pk"`
	SubmissionID int64  `xorm:"INDEX NOT NULL"`
	CaseIssueID  int64  `xorm:"INDEX NOT NULL"`
	ReporterID   int64  `xorm:"INDEX NOT NULL"`
	Reason       Reason `xorm:"VARCHAR(32) INDEX NOT NULL"`

	Verdict     Verdict `xorm:"VARCHAR(16) INDEX NOT NULL DEFAULT 'pending'"`
	VerdictBy   int64   `xorm:"NOT NULL DEFAULT 0"`
	VerdictUnix timeutil.TimeStamp
	OnHold      bool `xorm:"INDEX NOT NULL DEFAULT false"` // Story 2.7

	CreatedUnix timeutil.TimeStamp `xorm:"created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"updated"`
}

func (*Report) TableName() string { return "moderation_report" }

// InsertReport records the moderation data of a new reason issue. A report starts pending.
func InsertReport(ctx context.Context, r *Report) error {
	if r.Verdict == "" {
		// xorm inserts the zero value, so the column's default never applies
		r.Verdict = VerdictPending
	}
	return db.Insert(ctx, r)
}

// FindReportsBySubmission returns a submission's reports.
func FindReportsBySubmission(ctx context.Context, submissionID int64) ([]*Report, error) {
	reports := make([]*Report, 0, 2)
	return reports, db.GetEngine(ctx).Where(builder.Eq{"submission_id": submissionID}).Asc("issue_id").Find(&reports)
}

// FindReportsByCase returns a case's reports.
func FindReportsByCase(ctx context.Context, caseIssueID int64) ([]*Report, error) {
	reports := make([]*Report, 0, 4)
	return reports, db.GetEngine(ctx).Where(builder.Eq{"case_issue_id": caseIssueID}).Asc("issue_id").Find(&reports)
}

// FindPendingSubmissionsForReason returns the submissions behind the reporter's pending reports
// of a reason on a case: what the duplicate rule compares a new anchor with.
func FindPendingSubmissionsForReason(ctx context.Context, caseIssueID, reporterID int64, reason Reason) ([]*Submission, error) {
	subs := make([]*Submission, 0, 1)
	return subs, db.GetEngine(ctx).
		Where(builder.In("id", builder.Select("submission_id").From("moderation_report").Where(builder.Eq{
			"case_issue_id": caseIssueID,
			"reporter_id":   reporterID,
			"reason":        reason,
			"verdict":       VerdictPending,
		}))).
		Find(&subs)
}

// MoveReports moves every report of a case to another case (a merge).
func MoveReports(ctx context.Context, fromCaseIssueID, intoCaseIssueID int64) error {
	_, err := db.GetEngine(ctx).Where(builder.Eq{"case_issue_id": fromCaseIssueID}).
		Cols("case_issue_id").Update(&Report{CaseIssueID: intoCaseIssueID})
	return err
}
