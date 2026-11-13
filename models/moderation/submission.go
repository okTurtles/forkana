// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/modules/timeutil"

	"xorm.io/builder"
)

// AnchorStatus says what a submission's anchor points at.
type AnchorStatus string

const (
	// AnchorExact: SourceStart-SourceEnd is a block of the README at CommitSHA, checked by the
	// server.
	AnchorExact AnchorStatus = "exact"
	// AnchorQuoteOnly: a block that can't carry a source range (raw HTML, a rule); only the
	// quote and the section are kept.
	AnchorQuoteOnly AnchorStatus = "quote_only"
	// AnchorWhole: the whole article.
	AnchorWhole AnchorStatus = "whole"
)

// Submission is one send of the report form: one reporter, one anchor, one or more reasons
// (each a Report). The source offsets are bytes into the README at CommitSHA after
// end-of-line normalisation, front matter included; they are 0 unless the anchor is exact.
type Submission struct {
	ID          int64  `xorm:"pk autoincr"`
	ReporterID  int64  `xorm:"UNIQUE(reporter_client_token) INDEX NOT NULL"`
	ClientToken string `xorm:"UNIQUE(reporter_client_token) VARCHAR(36) NOT NULL"`
	CaseIssueID int64  `xorm:"INDEX NOT NULL"`

	CommitSHA     string       `xorm:"VARCHAR(64) NOT NULL DEFAULT ''"`
	TreePath      string       `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
	SourceStart   int64        `xorm:"NOT NULL DEFAULT 0"`
	SourceEnd     int64        `xorm:"NOT NULL DEFAULT 0"`
	Quote         string       `xorm:"TEXT"`
	SectionAnchor string       `xorm:"VARCHAR(255) NOT NULL DEFAULT ''"`
	AnchorStatus  AnchorStatus `xorm:"VARCHAR(16) NOT NULL DEFAULT 'whole'"`

	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
}

func (*Submission) TableName() string { return "moderation_submission" }

// InsertSubmission records a submission. A second submission with the same reporter and
// client token fails on the unique index.
func InsertSubmission(ctx context.Context, s *Submission) error {
	if s.AnchorStatus == "" {
		// xorm inserts the zero value, so the column's default never applies
		s.AnchorStatus = AnchorWhole
	}
	return db.Insert(ctx, s)
}

// GetSubmissionByToken returns the reporter's submission with the client token, or nil.
func GetSubmissionByToken(ctx context.Context, reporterID int64, token string) (*Submission, error) {
	s := new(Submission)
	has, err := db.GetEngine(ctx).Where(builder.Eq{"reporter_id": reporterID, "client_token": token}).Get(s)
	if err != nil || !has {
		return nil, err
	}
	return s, nil
}

// GetSubmission returns a submission by ID, or nil.
func GetSubmission(ctx context.Context, id int64) (*Submission, error) {
	s := new(Submission)
	has, err := db.GetEngine(ctx).ID(id).Get(s)
	if err != nil || !has {
		return nil, err
	}
	return s, nil
}

// FindSubmissionsByCase returns a case's submissions, oldest first.
func FindSubmissionsByCase(ctx context.Context, caseIssueID int64) ([]*Submission, error) {
	subs := make([]*Submission, 0, 4)
	return subs, db.GetEngine(ctx).Where(builder.Eq{"case_issue_id": caseIssueID}).Asc("id").Find(&subs)
}

// CountCaseSubmissions counts a case's submissions that have a report not on hold: the
// "three or more reports" priority rule counts submissions, not reasons.
func CountCaseSubmissions(ctx context.Context, caseIssueID int64) (int64, error) {
	return db.GetEngine(ctx).Table("moderation_submission").
		Where(builder.Eq{"case_issue_id": caseIssueID}).
		And(builder.In("id", builder.Select("submission_id").From("moderation_report").
			Where(builder.Eq{"case_issue_id": caseIssueID, "on_hold": false}))).
		Count()
}
