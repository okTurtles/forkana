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
	Section     string         `json:"section,omitempty"`
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
	// the part an exact report on block files under
	blockSection := block.SectionAnchor
	if blockSection == "" {
		blockSection = moderation_model.SectionIntro
	}
	openCase := func(t *testing.T, section string, reason moderation_model.Reason) *moderation_model.Case {
		t.Helper()
		c, err := moderation_model.FindOpenCase(t.Context(), article.ID, section, reason)
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
	t.Run("FilesOneCasePerReason", func(t *testing.T) {
		out := post(t, readerSession, reportBody{
			ClientToken: token, Anchor: "exact", CommitSHA: commitSHA, Range: blockRange,
			Reasons: []reportReason{{"doxxing", "Shares a home address."}, {"no-source", "No source for the claim."}},
		}, http.StatusOK)
		assert.Equal(t, "filed", out.Status)
		assert.Equal(t, []string{"doxxing", "no-source"}, out.Filed)
		assert.Equal(t, 4, countIssues(t), "two cases and two reason issues")

		sub, err := moderation_model.GetSubmission(t.Context(), out.SubmissionID)
		require.NoError(t, err)
		assert.Equal(t, moderation_model.AnchorExact, sub.AnchorStatus)
		assert.Equal(t, article.ID, sub.TargetRepoID)
		assert.Equal(t, int64(block.Start), sub.SourceStart)
		assert.Equal(t, block.Quote, sub.Quote, "the quote comes from the server's parse")

		reports, err := moderation_model.FindReportsBySubmission(t.Context(), sub.ID)
		require.NoError(t, err)
		require.Len(t, reports, 2)
		for _, r := range reports {
			c := openCase(t, blockSection, r.Reason)
			require.NotNil(t, c, "a case for %s", r.Reason)
			assert.Equal(t, c.IssueID, r.CaseIssueID)
			assert.Equal(t, moderation_model.CaseKindArticle, c.Kind)
			assert.Equal(t, article.OwnerID, c.TargetUserID)

			issue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: r.IssueID})
			assert.Equal(t, reader.ID, issue.PosterID)
			require.NoError(t, issue.LoadLabels(t.Context()))
			assert.ElementsMatch(t, []string{"type/report", r.Reason.LabelName()}, labelNames(issue.Labels))
			unittest.AssertExistsAndLoadBean(t, &issues_model.IssueDependency{IssueID: c.IssueID, DependencyID: r.IssueID})

			caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID})
			require.NoError(t, caseIssue.LoadLabels(t.Context()))
			assert.ElementsMatch(t, []string{"type/case", "kind/article", "source/reader", r.Reason.LabelName()}, labelNames(caseIssue.Labels))
			assert.Equal(t, moderation_service.PriorityNormal, caseIssue.Priority)
			assert.Contains(t, caseIssue.Title, string(r.Reason))

			audit, err := moderation_model.FindAuditByCase(t.Context(), c.IssueID)
			require.NoError(t, err)
			require.Len(t, audit, 1, "one entry per case")
			assert.Equal(t, moderation_model.AuditReportFiled, audit[0].Action)
			assert.Equal(t, moderation_model.ActorUser, audit[0].ActorRole)
			assert.Equal(t, r.Reason, audit[0].Reason)
			assert.Equal(t, string(r.Reason), audit[0].Summary)
			assert.NotContains(t, audit[0].Summary, "home address", "no typed text in the audit log")
		}
	})

	t.Run("CasePageHighlightsThePassage", func(t *testing.T) {
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: openCase(t, blockSection, moderation_model.ReasonDoxxing).IssueID})
		resp := loginUser(t, admin.Name).MakeRequest(t, NewRequest(t, "GET", fmt.Sprintf("/-/moderation/cases/%d", caseIssue.Index)), http.StatusOK)
		doc := NewHTMLParser(t, resp.Body)
		root := doc.Find(".moderation-passage-root")
		require.Equal(t, 1, root.Length())
		assert.Equal(t, blockRange, root.AttrOr("data-passage-highlight", ""))
		assert.Equal(t, 1, root.Find(fmt.Sprintf(`[data-source-range="%s"]`, blockRange)).Length(), "the reported block is in the rendered article")

		// a reason issue is in the cases repository too, but it isn't a case
		reports, err := moderation_model.FindReportsByCase(t.Context(), caseIssue.ID)
		require.NoError(t, err)
		require.NotEmpty(t, reports)
		reasonIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: reports[0].IssueID})
		loginUser(t, admin.Name).MakeRequest(t, NewRequest(t, "GET", fmt.Sprintf("/-/moderation/cases/%d", reasonIssue.Index)), http.StatusNotFound)
	})

	t.Run("RetryReturnsTheSameSubmission", func(t *testing.T) {
		before := countIssues(t)
		out := post(t, readerSession, reportBody{
			ClientToken: strings.ToUpper(token), Anchor: "whole",
			Reasons: []reportReason{{"something-else", "Changed my mind about the content."}},
		}, http.StatusOK)
		assert.Equal(t, "existing", out.Status)
		assert.Equal(t, []string{"doxxing", "no-source"}, out.Filed, "the first send wins")
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
		assert.Equal(t, before+2, countIssues(t), "a new case for the new reason, and its report")

		// the whole article is another part: the same reason opens another case
		out = post(t, readerSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"doxxing", "The whole article."}},
		}, http.StatusOK)
		assert.Equal(t, "filed", out.Status)
		whole := openCase(t, moderation_model.SectionWholeArticle, moderation_model.ReasonDoxxing)
		require.NotNil(t, whole)
		assert.NotEqual(t, openCase(t, blockSection, moderation_model.ReasonDoxxing).IssueID, whole.IssueID)
	})

	t.Run("ConcurrentSubmissionsLandOnOneCase", func(t *testing.T) {
		// The requests are built here and checked after Wait: require (FailNow) may only be
		// called from the test's goroutine.
		sessions := []*TestSession{readerSession, otherSession}
		reqs := make([]*RequestWrapper, len(sessions))
		for i, s := range sessions {
			reqs[i] = NewRequestWithJSON(t, "POST", reportURL, reportBody{
				ClientToken: uuid.NewString(), Anchor: "whole",
				Reasons: []reportReason{{"off-topic-or-spam", "Advertising."}},
			})
			reqs[i].SetHeader("X-Csrf-Token", GetUserCSRFToken(t, s))
		}
		codes := make([]int, len(sessions))
		var wg sync.WaitGroup
		for i, s := range sessions {
			wg.Go(func() {
				codes[i] = s.MakeRequest(t, reqs[i], NoExpectedStatus).Code
			})
		}
		wg.Wait()
		assert.Equal(t, []int{http.StatusOK, http.StatusOK}, codes)
		assert.Equal(t, 1, unittest.GetCount(t, &moderation_model.Case{
			TargetRepoID: article.ID, SectionAnchor: moderation_model.SectionWholeArticle, Reason: moderation_model.ReasonOffTopicOrSpam,
		}))
		c := openCase(t, moderation_model.SectionWholeArticle, moderation_model.ReasonOffTopicOrSpam)
		require.NotNil(t, c)
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID})
		assert.Equal(t, moderation_service.PriorityNormal, caseIssue.Priority, "two submissions")

		// three submissions on the case: High. Reports on the article's other cases don't count.
		post(t, loginUser(t, admin.Name), reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"off-topic-or-spam", "Advertising."}},
		}, http.StatusOK)
		caseIssue = unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID})
		assert.Equal(t, moderation_service.PriorityHigh, caseIssue.Priority)
	})

	t.Run("DeadlinePassedStaysOnTop", func(t *testing.T) {
		post(t, readerSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"something-else", "Unsourced."}},
		}, http.StatusOK)
		// Request sources was applied and its deadline passed (the decision service is 1.6)
		c := openCase(t, moderation_model.SectionWholeArticle, moderation_model.ReasonSomethingElse)
		require.NotNil(t, c)
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
		c := openCase(t, moderation_model.SectionWholeArticle, moderation_model.ReasonChildSafety)
		require.NotNil(t, c)
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID})
		assert.Equal(t, moderation_service.PriorityChildSafety, caseIssue.Priority)
	})

	// ChangePart works on the cases the subtests above filed: it can't run on its own.
	t.Run("ChangePart", func(t *testing.T) {
		ctx := t.Context()
		violence := openCase(t, blockSection, moderation_model.ReasonGraphicViolence)
		require.NotNil(t, violence)

		_, err := moderation_service.ChangeCasePart(ctx, reader, violence.IssueID, moderation_model.SectionWholeArticle)
		assert.Error(t, err, "only moderators")
		_, err = moderation_service.ChangeCasePart(ctx, admin, violence.IssueID, "no-such-heading")
		assert.Error(t, err, "a part the article doesn't have")

		// no whole-article case for the reason: the case moves
		res, err := moderation_service.ChangeCasePart(ctx, admin, violence.IssueID, moderation_model.SectionWholeArticle)
		require.NoError(t, err)
		assert.False(t, res.Merged)
		assert.Equal(t, violence.IssueID, res.Case.IssueID)
		assert.Nil(t, openCase(t, blockSection, moderation_model.ReasonGraphicViolence))
		moved := openCase(t, moderation_model.SectionWholeArticle, moderation_model.ReasonGraphicViolence)
		require.NotNil(t, moved)
		assert.Equal(t, violence.IssueID, moved.IssueID)
		unittest.AssertExistsAndLoadBean(t, &moderation_model.Audit{CaseIssueID: violence.IssueID, Action: moderation_model.AuditPartChanged})

		// a whole-article doxxing case exists: the passage's doxxing case merges into it
		doxxing := openCase(t, blockSection, moderation_model.ReasonDoxxing)
		into := openCase(t, moderation_model.SectionWholeArticle, moderation_model.ReasonDoxxing)
		require.NotNil(t, doxxing)
		require.NotNil(t, into)
		res, err = moderation_service.ChangeCasePart(ctx, admin, doxxing.IssueID, moderation_model.SectionWholeArticle)
		require.NoError(t, err)
		assert.True(t, res.Merged)
		assert.Equal(t, into.IssueID, res.Case.IssueID)

		merged, err := moderation_model.GetCase(ctx, doxxing.IssueID)
		require.NoError(t, err)
		assert.Equal(t, moderation_model.ResolutionMerged, merged.Resolution)
		assert.Equal(t, into.IssueID, merged.MergedIntoIssueID)
		mergedIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: doxxing.IssueID})
		assert.True(t, mergedIssue.IsClosed)
		require.NoError(t, mergedIssue.LoadLabels(ctx))
		assert.Contains(t, labelNames(mergedIssue.Labels), moderation_model.LabelResolutionMerged)
		left, err := moderation_model.FindReportsByCase(ctx, doxxing.IssueID)
		require.NoError(t, err)
		assert.Empty(t, left)
		reports, err := moderation_model.FindReportsByCase(ctx, into.IssueID)
		require.NoError(t, err)
		assert.Len(t, reports, 2, "its own report and the merged one")
		for _, r := range reports {
			unittest.AssertExistsAndLoadBean(t, &issues_model.IssueDependency{IssueID: into.IssueID, DependencyID: r.IssueID})
		}
		unittest.AssertNotExistsBean(t, &issues_model.IssueDependency{IssueID: doxxing.IssueID})
		unittest.AssertExistsAndLoadBean(t, &moderation_model.Audit{CaseIssueID: doxxing.IssueID, Action: moderation_model.AuditMerged})

		// a closed case can't change part
		_, err = moderation_service.ChangeCasePart(ctx, admin, doxxing.IssueID, blockSection)
		assert.Error(t, err)
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

	t.Run("QuoteOnly", func(t *testing.T) {
		out := post(t, readerSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "quote_only", CommitSHA: commitSHA,
			Quote: "  a passage\n in a table  ", Section: "user-content-no-such-heading",
			Reasons: []reportReason{{"hate-or-harassment", "Targets a person."}},
		}, http.StatusOK)
		assert.Equal(t, "filed", out.Status)
		sub, err := moderation_model.GetSubmission(t.Context(), out.SubmissionID)
		require.NoError(t, err)
		assert.Equal(t, moderation_model.AnchorQuoteOnly, sub.AnchorStatus)
		assert.Equal(t, "a passage in a table", sub.Quote, "whitespace collapsed")
		assert.Empty(t, sub.SectionAnchor, "a section the article doesn't have is dropped")
		assert.NotNil(t, openCase(t, moderation_model.SectionIntro, moderation_model.ReasonHateOrHarassment), "filed under the introduction")
	})

	t.Run("PrivateArticleIs404", func(t *testing.T) {
		_, err := db.GetEngine(t.Context()).ID(article.ID).Cols("is_private").Update(&repo_model.Repository{IsPrivate: true})
		require.NoError(t, err)
		defer func() {
			_, err := db.GetEngine(t.Context()).ID(article.ID).Cols("is_private").Update(&repo_model.Repository{IsPrivate: false})
			require.NoError(t, err)
		}()
		before := countIssues(t)
		post(t, readerSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"doxxing", "x"}},
		}, http.StatusNotFound)
		assert.Equal(t, before, countIssues(t))
	})

	t.Run("ReportersCantReadTheCasesRepository", func(t *testing.T) {
		c := openCase(t, blockSection, moderation_model.ReasonNoSource)
		require.NotNil(t, c)
		caseIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID})
		require.Equal(t, reader.ID, caseIssue.PosterID, "the reporter opened the case")
		reports, err := moderation_model.FindReportsByCase(t.Context(), c.IssueID)
		require.NoError(t, err)
		require.NotEmpty(t, reports)
		reasonIssue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: reports[0].IssueID})
		adminSession := loginUser(t, admin.Name)
		for _, index := range []int64{caseIssue.Index, reasonIssue.Index} {
			issueURL := fmt.Sprintf("/%s/%s/issues/%d", casesRepo.OwnerName, casesRepo.Name, index)
			adminSession.MakeRequest(t, NewRequest(t, "GET", issueURL), http.StatusOK)
			readerSession.MakeRequest(t, NewRequest(t, "GET", issueURL), http.StatusNotFound)
		}
	})

	t.Run("CrossReferencesAreNotCreated", func(t *testing.T) {
		before := unittest.GetCount(t, &issues_model.Comment{IssueID: 1})
		post(t, otherSession, reportBody{
			ClientToken: uuid.NewString(), Anchor: "whole",
			Reasons: []reportReason{{"plagiarism-or-copyright", "Copied, see user2/repo1#1 and #1."}},
		}, http.StatusOK)
		assert.Equal(t, before, unittest.GetCount(t, &issues_model.Comment{IssueID: 1}), "no reference comment on a public issue")

		// The case title quotes the article's subject and headings, which authors write. The
		// service is called directly: the report URL is looked up by the subject's name.
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: article.ID})
		require.NoError(t, repo.LoadOwner(t.Context()))
		require.NoError(t, repo.LoadSubject(t.Context()))
		repo.SubjectRelation.Name = "See user2/repo1#1 and #1"
		_, err := moderation_service.SubmitReport(t.Context(), moderation_service.ReportInput{
			Reporter: other, Repo: repo, ClientToken: uuid.NewString(), Anchor: moderation_model.AnchorWhole,
			Reasons: []moderation_service.ReasonInput{{Reason: moderation_model.ReasonNoSource, Explanation: "No sources."}},
		})
		require.NoError(t, err)
		c := openCase(t, moderation_model.SectionWholeArticle, moderation_model.ReasonNoSource)
		require.NotNil(t, c)
		assert.Contains(t, unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: c.IssueID}).Title, "user2/repo1#1")
		assert.Equal(t, before, unittest.GetCount(t, &issues_model.Comment{IssueID: 1}), "no reference comment from the case title")
		unittest.AssertNotExistsBean(t, &issues_model.Comment{RefIssueID: c.IssueID})
	})
}

func labelNames(labels []*issues_model.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}
