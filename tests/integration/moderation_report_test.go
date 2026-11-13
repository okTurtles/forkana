// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/markup/markdown"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/modules/timeutil"
	routers_moderation "code.gitea.io/gitea/routers/web/moderation"
	moderation_service "code.gitea.io/gitea/services/moderation"
	"code.gitea.io/gitea/tests"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reportReason struct {
	Reason      string `json:"reason"`
	Explanation string `json:"explanation"`
}

type reportBody struct {
	ClientToken string         `json:"client_token"`
	Anchor      string         `json:"anchor"`
	CommitSHA   string         `json:"commit_sha,omitempty"`
	Range       string         `json:"range,omitempty"`
	Quote       string         `json:"quote,omitempty"`
	Reasons     []reportReason `json:"reasons"`
}

func TestModerationReport(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	defer test.MockVariableValue(&setting.Moderation.Enabled, true)()

	admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	reader := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	other := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})
	article := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1}) // user2's article on example-subject
	require.NoError(t, article.LoadOwner(t.Context()))
	require.NoError(t, article.LoadSubject(t.Context()))
	reportURL := fmt.Sprintf("/subject/%s/%s/-/report", article.SubjectRelation.Slug, article.Owner.Name)

	_, err := moderation_service.EnsureSetup(t.Context(), moderation_service.SetupOptions{Owner: admin})
	require.NoError(t, err)
	casesRepo, err := moderation_service.GetCasesRepo(t.Context())
	require.NoError(t, err)

	gitRepo, err := gitrepo.OpenRepository(t.Context(), article)
	require.NoError(t, err)
	defer gitRepo.Close()
	commitSHA, err := gitRepo.GetBranchCommitID(article.DefaultBranch)
	require.NoError(t, err)
	commit, err := gitRepo.GetCommit(commitSHA)
	require.NoError(t, err)
	blob, err := commit.Tree.GetBlobByPath("README.md")
	require.NoError(t, err)
	readme, err := blob.GetBlobBytes(1 << 20)
	require.NoError(t, err)
	blocks := markdown.SourceBlocks(t.Context(), readme)
	require.NotEmpty(t, blocks)
	block := blocks[0]
	blockRange := fmt.Sprintf("%d-%d", block.Start, block.End)

	readerSession := loginUser(t, reader.Name)
	otherSession := loginUser(t, other.Name)
	post := func(t *testing.T, s *TestSession, body any, status int) *routers_moderation.ReportResponse {
		t.Helper()
		req := NewRequestWithJSON(t, "POST", reportURL, body)
		req.SetHeader("X-Csrf-Token", GetUserCSRFToken(t, s))
		resp := s.MakeRequest(t, req, status)
		if status != http.StatusOK {
			return nil
		}
		var out routers_moderation.ReportResponse
		DecodeJSON(t, resp, &out)
		return &out
	}
	openCase := func(t *testing.T) *moderation_model.Case {
		t.Helper()
		c, err := moderation_model.FindOpenCaseByTargetRepo(t.Context(), article.ID)
		require.NoError(t, err)
		return c
	}
	countIssues := func(t *testing.T) int {
		t.Helper()
		return unittest.GetCount(t, &issues_model.Issue{RepoID: casesRepo.ID})
	}

	t.Run("OffIs404", func(t *testing.T) {
		defer test.MockVariableValue(&setting.Moderation.Enabled, false)()
		req := NewRequestWithJSON(t, "POST", reportURL, reportBody{})
		req.SetHeader("X-Csrf-Token", GetUserCSRFToken(t, readerSession))
		readerSession.MakeRequest(t, req, http.StatusNotFound)
	})

	t.Run("SignedOutIsRefused", func(t *testing.T) {
		// a signed-out POST has no CSRF token, and is refused before it reaches the sign-in check
		MakeRequest(t, NewRequestWithJSON(t, "POST", reportURL, reportBody{}), http.StatusBadRequest)
	})

	token := uuid.NewString()
	t.Run("FilesOneIssuePerReason", func(t *testing.T) {
		out := post(t, readerSession, reportBody{
			ClientToken: token, Anchor: "exact", CommitSHA: commitSHA, Range: blockRange,
			Reasons: []reportReason{{"doxxing", "Shares a home address."}, {"no-reliable-source", "No source for the claim."}},
		}, http.StatusOK)
		assert.Equal(t, "filed", out.Status)
		assert.Equal(t, []string{"doxxing", "no-reliable-source"}, out.Filed)

		c := openCase(t)
		require.NotNil(t, c)
		assert.Equal(t, moderation_model.CaseKindArticle, c.Kind)
		assert.Equal(t, article.OwnerID, c.TargetUserID)
		assert.Equal(t, 3, countIssues(t), "the case and two reason issues")

		sub, err := moderation_model.GetSubmission(t.Context(), out.SubmissionID)
		require.NoError(t, err)
		assert.Equal(t, moderation_model.AnchorExact, sub.AnchorStatus)
		assert.Equal(t, int64(block.Start), sub.SourceStart)
		assert.Equal(t, block.Quote, sub.Quote, "the quote comes from the server's parse")

		reports, err := moderation_model.FindReportsBySubmission(t.Context(), sub.ID)
		require.NoError(t, err)
		require.Len(t, reports, 2)
		for _, r := range reports {
			issue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: r.IssueID})
			assert.Equal(t, reader.ID, issue.PosterID)
			require.NoError(t, issue.LoadLabels(t.Context()))
			names := labelNames(issue.Labels)
			assert.ElementsMatch(t, []string{"type/report", r.Reason.LabelName()}, names)
			unittest.AssertExistsAndLoadBean(t, &issues_model.IssueDependency{IssueID: c.IssueID, DependencyID: r.IssueID})
		}

		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID})
		require.NoError(t, caseIssue.LoadLabels(t.Context()))
		assert.ElementsMatch(t, []string{"type/case", "kind/article", "source/reader", "reason/doxxing", "reason/no-reliable-source"}, labelNames(caseIssue.Labels))
		assert.Equal(t, moderation_service.PriorityNormal, caseIssue.Priority)

		audit, err := moderation_model.FindAuditByCase(t.Context(), c.IssueID)
		require.NoError(t, err)
		require.Len(t, audit, 1, "one entry per submission")
		assert.Equal(t, moderation_model.AuditReportFiled, audit[0].Action)
		assert.Equal(t, moderation_model.ActorUser, audit[0].ActorRole)
		assert.Equal(t, "doxxing, no-reliable-source", audit[0].Summary)
		assert.NotContains(t, audit[0].Summary, "home address", "no typed text in the audit log")
	})

	t.Run("CasePageHighlightsThePassage", func(t *testing.T) {
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: openCase(t).IssueID})
		resp := loginUser(t, admin.Name).MakeRequest(t, NewRequest(t, "GET", fmt.Sprintf("/-/moderation/cases/%d", caseIssue.Index)), http.StatusOK)
		doc := NewHTMLParser(t, resp.Body)
		root := doc.Find(".moderation-passage-root")
		require.Equal(t, 1, root.Length())
		assert.Equal(t, blockRange, root.AttrOr("data-passage-highlight", ""))
		assert.Equal(t, 1, root.Find(fmt.Sprintf(`[data-source-range="%s"]`, blockRange)).Length(), "the reported block is in the rendered article")
	})

	t.Run("RetryReturnsTheSameSubmission", func(t *testing.T) {
		before := countIssues(t)
		out := post(t, readerSession, reportBody{
			ClientToken: strings.ToUpper(token), Anchor: "whole",
			Reasons: []reportReason{{"something-else", "Changed my mind about the content."}},
		}, http.StatusOK)
		assert.Equal(t, "existing", out.Status)
		assert.Equal(t, []string{"doxxing", "no-reliable-source"}, out.Filed, "the first send wins")
		assert.Equal(t, before, countIssues(t))
	})

	t.Run("RepeatsAreDropped", func(t *testing.T) {
		before := countIssues(t)
		out := post(t, readerSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "exact", CommitSHA: commitSHA, Range: blockRange,
			Reasons: []reportReason{{"doxxing", "Again."}},
		}, http.StatusOK)
		assert.Equal(t, "already_reported", out.Status)
		assert.Equal(t, before, countIssues(t))

		out = post(t, readerSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "exact", CommitSHA: commitSHA, Range: blockRange,
			Reasons: []reportReason{{"doxxing", "Again."}, {"graphic-violence", "New reason."}},
		}, http.StatusOK)
		assert.Equal(t, "filed", out.Status)
		assert.Equal(t, []string{"graphic-violence"}, out.Filed)
		assert.Equal(t, []string{"doxxing"}, out.Repeated)
		assert.Equal(t, before+1, countIssues(t))

		// the whole article isn't the same passage
		out = post(t, readerSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"doxxing", "The whole article."}},
		}, http.StatusOK)
		assert.Equal(t, "filed", out.Status)
	})

	t.Run("ConcurrentSubmissionsLandOnOneCase", func(t *testing.T) {
		before := openCase(t)
		var wg sync.WaitGroup
		for _, s := range []*TestSession{readerSession, otherSession} {
			wg.Add(1)
			go func(s *TestSession) {
				defer wg.Done()
				post(t, s, reportBody{
					ClientToken: uuid.NewString(), Anchor: "whole",
					Reasons: []reportReason{{"off-topic-or-spam", "Advertising."}},
				}, http.StatusOK)
			}(s)
		}
		wg.Wait()
		assert.Equal(t, 1, unittest.GetCount(t, &moderation_model.Case{TargetRepoID: article.ID}))
		assert.Equal(t, before.IssueID, openCase(t).IssueID)

		// three submissions or more: High
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: before.IssueID})
		assert.Equal(t, moderation_service.PriorityHigh, caseIssue.Priority)
	})

	t.Run("DeadlinePassedStaysOnTop", func(t *testing.T) {
		// Request sources was applied and its deadline passed (the decision service is 1.6)
		c := openCase(t)
		_, err := db.GetEngine(t.Context()).ID(c.IssueID).Cols("decision").Update(&moderation_model.Case{Decision: moderation_model.DecisionSourcesRequested})
		require.NoError(t, err)
		_, err = db.GetEngine(t.Context()).ID(c.IssueID).Cols("deadline_unix").Update(&issues_model.Issue{DeadlineUnix: timeutil.TimeStampNow() - 60})
		require.NoError(t, err)

		post(t, otherSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"something-else", "Still unsourced."}},
		}, http.StatusOK)
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID})
		assert.Equal(t, moderation_service.PriorityDeadlinePassed, caseIssue.Priority, "a new report doesn't lower it")
	})

	t.Run("ChildSafetyFirst", func(t *testing.T) {
		post(t, otherSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"child-safety", "Endangers minors."}},
		}, http.StatusOK)
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: openCase(t).IssueID})
		assert.Equal(t, moderation_service.PriorityChildSafety, caseIssue.Priority)
	})

	t.Run("InvalidReports", func(t *testing.T) {
		before := countIssues(t)
		for name, body := range map[string]reportBody{
			"NotAUUID":         {ClientToken: "abc", Anchor: "whole", Reasons: []reportReason{{"doxxing", "x"}}},
			"NoReason":         {ClientToken: uuid.NewString(), Anchor: "whole"},
			"UnknownReason":    {ClientToken: uuid.NewString(), Anchor: "whole", Reasons: []reportReason{{"rude", "x"}}},
			"NoExplanation":    {ClientToken: uuid.NewString(), Anchor: "whole", Reasons: []reportReason{{"doxxing", "  "}}},
			"ForgedRange":      {ClientToken: uuid.NewString(), Anchor: "exact", CommitSHA: commitSHA, Range: fmt.Sprintf("%d-%d", block.Start, block.End-1), Reasons: []reportReason{{"doxxing", "x"}}},
			"MalformedRange":   {ClientToken: uuid.NewString(), Anchor: "exact", CommitSHA: commitSHA, Range: "a-b", Reasons: []reportReason{{"doxxing", "x"}}},
			"UnknownCommit":    {ClientToken: uuid.NewString(), Anchor: "exact", CommitSHA: strings.Repeat("0", 40), Range: blockRange, Reasons: []reportReason{{"doxxing", "x"}}},
			"QuoteOnlyNoQuote": {ClientToken: uuid.NewString(), Anchor: "quote_only", CommitSHA: commitSHA, Reasons: []reportReason{{"doxxing", "x"}}},
		} {
			t.Run(name, func(t *testing.T) {
				post(t, readerSession, body, http.StatusUnprocessableEntity)
			})
		}
		assert.Equal(t, before, countIssues(t))
	})

	t.Run("CrossReferencesAreNotCreated", func(t *testing.T) {
		before := unittest.GetCount(t, &issues_model.Comment{IssueID: 1})
		post(t, otherSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"plagiarism-or-copyright", "Copied, see user2/repo1#1 and #1."}},
		}, http.StatusOK)
		assert.Equal(t, before, unittest.GetCount(t, &issues_model.Comment{IssueID: 1}), "no reference comment on a public issue")
	})
}

func labelNames(labels []*issues_model.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}
