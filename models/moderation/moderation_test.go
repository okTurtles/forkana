// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation_test

import (
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModerationRecords(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// issue 1 is open, issue 5 is closed (fixtures)
	open := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: 1})
	require.False(t, open.IsClosed)
	closed := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: 5})
	require.True(t, closed.IsClosed)

	require.NoError(t, moderation_model.InsertCase(ctx, &moderation_model.Case{IssueID: closed.ID, Kind: moderation_model.CaseKindArticle, TargetRepoID: 42, TargetUserID: 2}))
	c, err := moderation_model.FindOpenCaseByTargetRepo(ctx, 42)
	require.NoError(t, err)
	assert.Nil(t, c, "a closed case isn't the open case")

	require.NoError(t, moderation_model.InsertCase(ctx, &moderation_model.Case{IssueID: open.ID, Kind: moderation_model.CaseKindArticle, TargetRepoID: 42, TargetUserID: 2}))
	c, err = moderation_model.FindOpenCaseByTargetRepo(ctx, 42)
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, open.ID, c.IssueID)
	assert.Equal(t, "kind/article", c.Kind.LabelName())

	got, err := moderation_model.GetCase(ctx, open.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(42), got.TargetRepoID)
	got, err = moderation_model.GetCase(ctx, 999)
	require.NoError(t, err)
	assert.Nil(t, got)

	// submissions: the client token is unique per reporter
	sub := &moderation_model.Submission{ReporterID: 4, ClientToken: "0b7a6b54-0d5f-4c1e-8f5e-2a1f3c4d5e6f", CaseIssueID: open.ID, AnchorStatus: moderation_model.AnchorWhole}
	require.NoError(t, moderation_model.InsertSubmission(ctx, sub))
	require.Error(t, moderation_model.InsertSubmission(ctx, &moderation_model.Submission{ReporterID: 4, ClientToken: sub.ClientToken, CaseIssueID: open.ID}))
	require.NoError(t, moderation_model.InsertSubmission(ctx, &moderation_model.Submission{ReporterID: 5, ClientToken: sub.ClientToken, CaseIssueID: open.ID}))

	byToken, err := moderation_model.GetSubmissionByToken(ctx, 4, sub.ClientToken)
	require.NoError(t, err)
	assert.Equal(t, sub.ID, byToken.ID)
	byToken, err = moderation_model.GetSubmissionByToken(ctx, 2, sub.ClientToken)
	require.NoError(t, err)
	assert.Nil(t, byToken, "another reporter's token returns nothing")

	// reports
	require.NoError(t, moderation_model.InsertReport(ctx, &moderation_model.Report{IssueID: 2, SubmissionID: sub.ID, CaseIssueID: open.ID, ReporterID: 4, Reason: moderation_model.ReasonDoxxing}))
	require.NoError(t, moderation_model.InsertReport(ctx, &moderation_model.Report{IssueID: 3, SubmissionID: sub.ID, CaseIssueID: open.ID, ReporterID: 4, Reason: moderation_model.ReasonNoReliableSource}))
	reports, err := moderation_model.FindReportsBySubmission(ctx, sub.ID)
	require.NoError(t, err)
	require.Len(t, reports, 2)
	assert.Equal(t, moderation_model.VerdictPending, reports[0].Verdict)

	reasons, err := moderation_model.CaseReasons(ctx, open.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []moderation_model.Reason{moderation_model.ReasonDoxxing, moderation_model.ReasonNoReliableSource}, reasons)

	n, err := moderation_model.CountCaseSubmissions(ctx, open.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "the submission without reports isn't counted")

	pending, err := moderation_model.FindPendingSubmissionsForReason(ctx, open.ID, 4, moderation_model.ReasonDoxxing)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, sub.ID, pending[0].ID)
	pending, err = moderation_model.FindPendingSubmissionsForReason(ctx, open.ID, 4, moderation_model.ReasonSomethingElse)
	require.NoError(t, err)
	assert.Empty(t, pending)

	// audit
	require.NoError(t, moderation_model.InsertAudit(ctx, &moderation_model.Audit{ActorID: 4, ActorRole: moderation_model.ActorUser, Action: moderation_model.AuditReportFiled, CaseIssueID: open.ID, TargetRepoID: 42, TargetUserID: 2}))
	entries, err := moderation_model.FindAuditByCase(ctx, open.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.NotZero(t, entries[0].CreatedUnix)
}

func TestCaseDecisionsMatchLabels(t *testing.T) {
	for _, d := range []string{moderation_model.DecisionDismissed, moderation_model.DecisionSourcesRequested, moderation_model.DecisionRemoved, moderation_model.DecisionEscalated} {
		found := false
		for _, l := range moderation_model.CoreLabels() {
			found = found || l.Name == "decision/"+d
		}
		assert.True(t, found, "decision %q has a label", d)
	}
}

func TestCaseWaitAndReopen(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	require.NoError(t, moderation_model.InsertCase(ctx, &moderation_model.Case{IssueID: 1, Kind: moderation_model.CaseKindArticle, TargetRepoID: 1, AssignedUnix: 100}))
	require.NoError(t, moderation_model.SetWaitingOnOwner(ctx, 1, true, 500))
	c, err := moderation_model.GetCase(ctx, 1)
	require.NoError(t, err)
	assert.True(t, c.WaitingOnOwner)
	assert.Equal(t, timeutil.TimeStamp(100), c.AssignedUnix, "starting the wait keeps the timer")

	require.NoError(t, moderation_model.SetWaitingOnOwner(ctx, 1, false, 900))
	c, err = moderation_model.GetCase(ctx, 1)
	require.NoError(t, err)
	assert.False(t, c.WaitingOnOwner)
	assert.Equal(t, timeutil.TimeStamp(900), c.AssignedUnix, "ending the wait restarts the 48h timer")

	assert.False(t, (&moderation_model.Case{DecidedUnix: 10}).IsReexamining())
	assert.True(t, (&moderation_model.Case{DecidedUnix: 10, ReopenedUnix: 20}).IsReexamining())
	assert.False(t, (&moderation_model.Case{DecidedUnix: 30, ReopenedUnix: 20}).IsReexamining(), "decided again after the reopen")
}
