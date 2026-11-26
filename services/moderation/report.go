// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/globallock"
	issue_indexer "code.gitea.io/gitea/modules/indexer/issues"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/markup/markdown"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"

	"github.com/google/uuid"
	"xorm.io/builder"
)

const (
	// ArticleTreePath is the file an article is made of.
	ArticleTreePath = "README.md"
	// MaxExplanationLength caps a reason's explanation, in runes.
	MaxExplanationLength = 2000
	// maxReadmeSize caps how much of a README is read to check an anchor.
	maxReadmeSize = 10 << 20
)

// Case priorities, stored in Issue.Priority for the queue's sort (higher first).
const (
	PriorityNormal         = 0
	PriorityHigh           = 1 // three or more submissions on the case
	PriorityDeadlinePassed = 2 // Request sources, and the deadline passed: on top for its moderator, or of the Queue if nobody holds it
	PriorityChildSafety    = 3 // a child-safety case: reviewed first
	highPrioritySubmits    = 3
)

// ReasonInput is one ticked reason with its explanation.
type ReasonInput struct {
	Reason      moderation_model.Reason
	Explanation string
}

// ReportInput is one send of the report form on an article.
type ReportInput struct {
	Reporter    *user_model.User
	Repo        *repo_model.Repository
	ClientToken string
	// Anchor is what the report is about. For AnchorExact, CommitSHA and SourceStart/End are
	// the revision the reader saw and the block's data-source-range; for AnchorQuoteOnly,
	// CommitSHA and Quote (and optionally Section); AnchorWhole needs neither.
	Anchor      moderation_model.AnchorStatus
	CommitSHA   string
	SourceStart int
	SourceEnd   int
	Quote       string
	Section     string
	Reasons     []ReasonInput
}

// ReportResult is what SubmitReport did.
type ReportResult struct {
	// Submission is nil when nothing was written (every reason was a repeat).
	Submission *moderation_model.Submission
	// Existing is set when the client token had already been used: nothing new was written.
	Existing bool
	// Filed are the reasons this call filed; Repeated those dropped by the duplicate rule.
	Filed    []moderation_model.Reason
	Repeated []moderation_model.Reason
}

// ErrInvalidReport is returned (wrapped, with what is wrong) for input that can't be filed.
var ErrInvalidReport = util.NewInvalidArgumentErrorf("invalid report")

// ErrTokenReused is returned when a client token comes back with another target.
var ErrTokenReused = errors.New("client token already used for another target")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidReport, fmt.Sprintf(format, args...))
}

// SubmitReport files a report on an article: one reason issue per ticked reason in the cases
// repository, each in the open case for that reason on the reported part (created when there
// is none), in one transaction with an audit entry per case (foundations decisions 2–4, 6, 8,
// 13).
//
// A client token that was already used returns that submission unchanged. A ticked reason the
// reporter already reported, still pending, on an overlapping anchor is dropped; when every
// reason is, nothing is written and Filed is empty.
func SubmitReport(ctx context.Context, in ReportInput) (*ReportResult, error) {
	if !setting.Moderation.Enabled {
		return nil, invalid("moderation is not enabled")
	}
	if err := validateReportInput(&in); err != nil {
		return nil, err
	}

	// A retry: answer before taking any lock.
	if existing, err := existingSubmission(ctx, in); existing != nil || err != nil {
		return existing, err
	}

	anchor, err := resolveAnchor(ctx, in)
	if err != nil {
		return nil, err
	}

	casesRepo, err := GetCasesRepo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get cases repository: %w", err)
	}
	labels, err := coreLabelsByName(ctx, casesRepo.OwnerID)
	if err != nil {
		return nil, err
	}

	// One open case per (article, part, reason): the lock serialises find-or-create across
	// requests (and across processes with a redis global lock); the transaction re-checks. It
	// is per article, not per case, because Change part and a whole-article removal touch
	// several of the article's cases.
	release, err := globallock.Lock(ctx, fmt.Sprintf("moderation:case:repo:%d", in.Repo.ID))
	if err != nil {
		return nil, fmt.Errorf("lock the article's case: %w", err)
	}
	defer release()

	// The same token may have been waiting on the lock: answer with what the first send filed,
	// rather than finding its reasons repeated.
	if existing, err := existingSubmission(ctx, in); existing != nil || err != nil {
		return existing, err
	}

	section := caseSection(in.Anchor, anchor.section)
	var result *ReportResult
	var indexIssueIDs []int64
	err = db.WithTx(ctx, func(ctx context.Context) error {
		result, indexIssueIDs = &ReportResult{}, nil

		// The open case of each ticked reason, if any, and the reasons to file.
		cases := make(map[moderation_model.Reason]*moderation_model.Case, len(in.Reasons))
		for _, r := range in.Reasons {
			c, err := moderation_model.FindOpenCase(ctx, in.Repo.ID, section, r.Reason)
			if err != nil {
				return fmt.Errorf("find the %s case: %w", r.Reason, err)
			}
			if c != nil {
				repeat, err := isRepeat(ctx, c.IssueID, in, r.Reason, anchor)
				if err != nil {
					return err
				}
				if repeat {
					result.Repeated = append(result.Repeated, r.Reason)
					continue
				}
			}
			cases[r.Reason] = c
			result.Filed = append(result.Filed, r.Reason)
		}
		if len(result.Filed) == 0 {
			return nil
		}

		sub := &moderation_model.Submission{
			ReporterID:    in.Reporter.ID,
			ClientToken:   in.ClientToken,
			TargetRepoID:  in.Repo.ID,
			CommitSHA:     anchor.commitSHA,
			TreePath:      ArticleTreePath,
			SourceStart:   int64(anchor.start),
			SourceEnd:     int64(anchor.end),
			Quote:         anchor.quote,
			SectionAnchor: anchor.section,
			AnchorStatus:  in.Anchor,
		}
		if err := moderation_model.InsertSubmission(ctx, sub); err != nil {
			return fmt.Errorf("insert submission: %w", err)
		}
		result.Submission = sub

		for _, r := range in.Reasons {
			if !slices.Contains(result.Filed, r.Reason) {
				continue
			}
			c, caseIssue, err := loadOrCreateCase(ctx, casesRepo, in, cases[r.Reason], section, anchor.sectionTitle, r.Reason, labels)
			if err != nil {
				return err
			}
			// A new case, or an existing one whose labels and priority fileReport may change.
			indexIssueIDs = append(indexIssueIDs, caseIssue.ID)
			issue, err := fileReport(ctx, casesRepo, in, sub, c, caseIssue, r, labels)
			if err != nil {
				return err
			}
			indexIssueIDs = append(indexIssueIDs, issue.ID)
		}
		return nil
	})
	if err != nil {
		// Two sends of one token racing: the loser's insert fails on the unique index. Its
		// transaction is rolled back; answer with the winner's submission. The lock is per
		// article, so the winner may have used the token on another article.
		existing, lookupErr := existingSubmission(ctx, in)
		if errors.Is(lookupErr, ErrTokenReused) {
			return nil, lookupErr
		} else if lookupErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}

	// After the commit only: the indexer, so the queue's label filters find the issues. No
	// mention parsing and no stock notifications (foundations decisions 8).
	for _, id := range indexIssueIDs {
		issue_indexer.UpdateIssueIndexer(ctx, id)
	}
	return result, nil
}

func validateReportInput(in *ReportInput) error {
	if in.Reporter == nil || in.Reporter.ID <= 0 || in.Repo == nil {
		return invalid("no reporter or article")
	}
	if in.Repo.IsModerationCasesRepo() {
		return invalid("the cases repository can't be reported")
	}
	if _, err := uuid.Parse(in.ClientToken); err != nil || len(in.ClientToken) != 36 {
		return invalid("client token must be a UUID")
	}
	in.ClientToken = strings.ToLower(in.ClientToken)
	if len(in.Reasons) == 0 {
		return invalid("no reason ticked")
	}
	seen := map[moderation_model.Reason]bool{}
	for i, r := range in.Reasons {
		if !r.Reason.IsValid() {
			return invalid("unknown reason %q", r.Reason)
		}
		if seen[r.Reason] {
			return invalid("reason %q ticked twice", r.Reason)
		}
		seen[r.Reason] = true
		in.Reasons[i].Explanation = strings.TrimSpace(r.Explanation)
		if in.Reasons[i].Explanation == "" {
			return invalid("reason %q needs an explanation", r.Reason)
		}
		if utf8.RuneCountInString(in.Reasons[i].Explanation) > MaxExplanationLength {
			return invalid("explanation of %q is longer than %d characters", r.Reason, MaxExplanationLength)
		}
	}
	switch in.Anchor {
	case moderation_model.AnchorWhole, moderation_model.AnchorExact, moderation_model.AnchorQuoteOnly:
	default:
		return invalid("unknown anchor %q", in.Anchor)
	}
	return nil
}

// existingSubmission returns the result for a client token that was already used, or nil.
func existingSubmission(ctx context.Context, in ReportInput) (*ReportResult, error) {
	sub, err := moderation_model.GetSubmissionByToken(ctx, in.Reporter.ID, in.ClientToken)
	if err != nil || sub == nil {
		return nil, err
	}
	if sub.TargetRepoID != in.Repo.ID {
		return nil, ErrTokenReused
	}
	reports, err := moderation_model.FindReportsBySubmission(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	filed := make([]moderation_model.Reason, len(reports))
	for i, r := range reports {
		filed[i] = r.Reason
	}
	return &ReportResult{Submission: sub, Existing: true, Filed: filed}, nil
}

type resolvedAnchor struct {
	commitSHA  string
	start, end int
	quote      string
	section    string
	// sectionTitle is the text of the heading section names, for the case title.
	sectionTitle string
}

// resolveAnchor checks the anchor against the README at the revision the reader saw. The
// browser's range is never trusted: the quote and the section come from the server's parse.
func resolveAnchor(ctx context.Context, in ReportInput) (*resolvedAnchor, error) {
	if in.Anchor == moderation_model.AnchorWhole {
		return &resolvedAnchor{}, nil
	}
	if !isFullCommitID(in.CommitSHA) {
		return nil, invalid("commit must be a full commit ID")
	}
	raw, err := readArticleAt(ctx, in.Repo, in.CommitSHA)
	if err != nil {
		return nil, err
	}
	blocks := markdown.SourceBlocks(ctx, raw)
	if in.Anchor == moderation_model.AnchorExact {
		block, ok := markdown.FindSourceRange(blocks, len(raw), in.SourceStart, in.SourceEnd)
		if !ok {
			return nil, invalid("the passage isn't a block of the article at %s", in.CommitSHA)
		}
		if err := checkSectionAnchor(block.SectionAnchor); err != nil {
			return nil, err
		}
		return &resolvedAnchor{
			commitSHA: in.CommitSHA, start: block.Start, end: block.End,
			quote: block.Quote, section: block.SectionAnchor,
			sectionTitle: headingTitle(blocks, block.SectionAnchor),
		}, nil
	}

	// quote_only: a block that can't carry a range. Keep the quote (capped) and a section only
	// if it is one of the article's headings.
	quote := strings.Join(strings.Fields(in.Quote), " ")
	if quote == "" {
		return nil, invalid("a quote-only passage needs its quote")
	}
	if utf8.RuneCountInString(quote) > markdown.MaxSourceBlockQuote {
		quote = string([]rune(quote)[:markdown.MaxSourceBlockQuote])
	}
	section, title := "", ""
	if in.Section != "" && checkSectionAnchor(in.Section) == nil {
		if title = headingTitle(blocks, in.Section); title != "" {
			section = in.Section
		}
	}
	return &resolvedAnchor{commitSHA: in.CommitSHA, quote: quote, section: section, sectionTitle: title}, nil
}

// headingTitle returns the text of the article's heading with the id, or "" when there is
// none (no id: the passage is before the first heading). It is shortened to fit
// Case.SectionTitle and the case title.
func headingTitle(blocks []markdown.SourceBlock, id string) string {
	if id == "" {
		return ""
	}
	for _, b := range blocks {
		if b.Kind == "heading" && b.SectionAnchor == id {
			if b.Quote == "" {
				return id
			}
			return util.EllipsisDisplayString(b.Quote, moderation_model.MaxSectionLength)
		}
	}
	return ""
}

// checkSectionAnchor refuses a heading id that doesn't fit Case.SectionAnchor. Ids are slugs
// of the whole heading, so a very long heading has one; it isn't shortened, because the case
// key compares it with the article's ids. The whole article can still be reported.
func checkSectionAnchor(id string) error {
	if utf8.RuneCountInString(id) > moderation_model.MaxSectionLength {
		return invalid("the passage's section heading is too long; report the whole article instead")
	}
	return nil
}

// caseSection is the part of the article a report is filed under (Case.SectionAnchor): the
// whole article, the passage's section, or the introduction before the first heading.
func caseSection(status moderation_model.AnchorStatus, passageSection string) string {
	switch {
	case status == moderation_model.AnchorWhole:
		return moderation_model.SectionWholeArticle
	case passageSection == "":
		return moderation_model.SectionIntro
	default:
		return passageSection
	}
}

func isFullCommitID(sha string) bool {
	if len(sha) != 40 && len(sha) != 64 {
		return false
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// readArticleAt reads the article's README at a commit of its repository. A commit or README
// that doesn't exist is an invalid report; other failures are server errors.
func readArticleAt(ctx context.Context, repo *repo_model.Repository, sha string) ([]byte, error) {
	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	defer gitRepo.Close()
	commit, err := gitRepo.GetCommit(sha)
	if errors.Is(err, util.ErrNotExist) {
		return nil, invalid("unknown commit %s", sha)
	} else if err != nil {
		return nil, fmt.Errorf("get commit %s: %w", sha, err)
	}
	return readArticleOf(commit)
}

// readArticleOf reads the article's README in a commit. A README larger than maxReadmeSize is
// refused rather than cut, so that ranges past the cut aren't reported as unknown blocks.
func readArticleOf(commit *git.Commit) ([]byte, error) {
	blob, err := commit.Tree.GetBlobByPath(ArticleTreePath)
	if errors.Is(err, util.ErrNotExist) {
		return nil, invalid("the article has no %s at %s", ArticleTreePath, commit.ID)
	} else if err != nil {
		return nil, fmt.Errorf("get %s at %s: %w", ArticleTreePath, commit.ID, err)
	}
	if blob.Size() > maxReadmeSize {
		return nil, invalid("the article at %s is too large to check a passage in", commit.ID)
	}
	raw, err := blob.GetBlobBytes(maxReadmeSize)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ArticleTreePath, err)
	}
	return raw, nil
}

func coreLabelsByName(ctx context.Context, orgID int64) (map[string]*issues_model.Label, error) {
	labels, err := issues_model.GetLabelsByOrgID(ctx, orgID, "", db.ListOptionsAll)
	if err != nil {
		return nil, fmt.Errorf("get moderation labels: %w", err)
	}
	byName := make(map[string]*issues_model.Label, len(labels))
	for _, l := range labels {
		byName[l.Name] = l
	}
	for _, l := range moderation_model.CoreLabels() {
		if byName[l.Name] == nil {
			return nil, fmt.Errorf("moderation label %q is missing: run 'gitea admin moderation setup'", l.Name)
		}
	}
	return byName, nil
}

// loadOrCreateCase returns the case issue of c, or creates the case for the reason on the
// part when c is nil. The caller holds the article's lock and found c in its transaction.
func loadOrCreateCase(ctx context.Context, casesRepo *repo_model.Repository, in ReportInput, c *moderation_model.Case, section, sectionTitle string, reason moderation_model.Reason, labels map[string]*issues_model.Label) (*moderation_model.Case, *issues_model.Issue, error) {
	if c != nil {
		issue, err := issues_model.GetIssueByID(ctx, c.IssueID)
		if err != nil {
			return nil, nil, fmt.Errorf("get case issue: %w", err)
		}
		issue.Repo = casesRepo
		return c, issue, nil
	}

	kind := moderation_model.CaseKindArticle
	if in.Repo.IsFork {
		kind = moderation_model.CaseKindFork
	}
	if err := in.Repo.LoadOwner(ctx); err != nil {
		return nil, nil, fmt.Errorf("load article owner: %w", err)
	}
	if err := in.Repo.LoadSubject(ctx); err != nil {
		return nil, nil, fmt.Errorf("load article subject: %w", err)
	}

	// The case's poster is the reporter who opened it: an issue needs one, and only
	// moderators can read the cases repository.
	issue := &issues_model.Issue{
		RepoID:   casesRepo.ID,
		Repo:     casesRepo,
		Title:    "case",
		PosterID: in.Reporter.ID,
		Poster:   in.Reporter,
	}
	labelIDs := []int64{
		labels[moderation_model.LabelTypeCase].ID, labels[kind.LabelName()].ID,
		labels[moderation_model.LabelSourceReader].ID, labels[reason.LabelName()].ID,
	}
	if err := issues_model.NewIssue(ctx, casesRepo, issue, labelIDs, nil); err != nil {
		return nil, nil, fmt.Errorf("create case issue: %w", err)
	}
	// The title is set afterwards: NewIssue adds cross-references from the title as well as
	// the content (foundations decisions 8), and the title quotes the article's heading.
	issue.Title = caseTitle(in.Repo, section, sectionTitle, reason)
	if err := issues_model.UpdateIssueCols(ctx, issue, "name"); err != nil {
		return nil, nil, fmt.Errorf("set case title: %w", err)
	}

	previous, err := previousCaseIssueID(ctx, in.Repo.ID, section, reason)
	if err != nil {
		return nil, nil, err
	}
	c = &moderation_model.Case{
		IssueID:             issue.ID,
		Kind:                kind,
		TargetRepoID:        in.Repo.ID,
		SectionAnchor:       section,
		SectionTitle:        sectionTitle,
		Reason:              reason,
		TargetUserID:        in.Repo.OwnerID,
		PreviousCaseIssueID: previous,
	}
	if err := moderation_model.InsertCase(ctx, c); err != nil {
		return nil, nil, fmt.Errorf("insert case: %w", err)
	}
	return c, issue, nil
}

// caseTitle names a case in the stock issue list: the article, the part and the reason.
func caseTitle(repo *repo_model.Repository, section, sectionTitle string, reason moderation_model.Reason) string {
	article := repo.Owner.Name + " / " + repo.Name
	if repo.SubjectRelation != nil {
		article = repo.Owner.Name + " / " + repo.SubjectRelation.Name
	}
	part := sectionTitle
	switch section {
	case moderation_model.SectionWholeArticle:
		part = "whole article"
	case moderation_model.SectionIntro:
		part = "introduction"
	}
	// Issue titles are VARCHAR(255) too.
	return util.EllipsisDisplayString(article+" · "+part+" · "+string(reason), 255)
}

// fileReport files one reason of a submission in its case: the reason issue, the report,
// the case's dependency on it, the case's labels and priority, and the audit entry.
func fileReport(ctx context.Context, casesRepo *repo_model.Repository, in ReportInput, sub *moderation_model.Submission, c *moderation_model.Case, caseIssue *issues_model.Issue, r ReasonInput, labels map[string]*issues_model.Label) (*issues_model.Issue, error) {
	issue, err := createReasonIssue(ctx, casesRepo, in, r, labels)
	if err != nil {
		return nil, err
	}
	if err := moderation_model.InsertReport(ctx, &moderation_model.Report{
		IssueID:      issue.ID,
		SubmissionID: sub.ID,
		CaseIssueID:  c.IssueID,
		ReporterID:   in.Reporter.ID,
		Reason:       r.Reason,
	}); err != nil {
		return nil, fmt.Errorf("insert report: %w", err)
	}
	// The case is blocked by each of its reports: the native dependency sidebar, and the
	// guard that stops a case closing before every report is decided.
	if err := issues_model.CreateIssueDependency(ctx, in.Reporter, caseIssue, issue); err != nil {
		return nil, fmt.Errorf("link report to case: %w", err)
	}
	if err := syncCaseLabels(ctx, caseIssue, c, labels, in.Reporter); err != nil {
		return nil, err
	}
	if err := updateCasePriority(ctx, caseIssue, c); err != nil {
		return nil, err
	}
	// One entry per case the submission files into, which is one per reason.
	if err := moderation_model.InsertAudit(ctx, &moderation_model.Audit{
		ActorID:      in.Reporter.ID,
		ActorRole:    moderation_model.ActorUser,
		Action:       moderation_model.AuditReportFiled,
		CaseIssueID:  c.IssueID,
		TargetUserID: c.TargetUserID,
		TargetRepoID: c.TargetRepoID,
		Reason:       r.Reason,
		Summary:      string(r.Reason),
	}); err != nil {
		return nil, fmt.Errorf("insert audit entry: %w", err)
	}
	return issue, nil
}

// previousCaseIssueID returns the newest closed case for the reason on the part, or 0. A case
// merged into another isn't one: its reports live on in the case it was merged into.
func previousCaseIssueID(ctx context.Context, repoID int64, section string, reason moderation_model.Reason) (int64, error) {
	var id int64
	_, err := db.GetEngine(ctx).Table("moderation_case").
		Join("INNER", "issue", "issue.id = moderation_case.issue_id").
		Where(builder.Eq{
			"moderation_case.target_repo_id": repoID,
			"moderation_case.section_anchor": section,
			"moderation_case.reason":         reason,
			"issue.is_closed":                true,
		}.And(builder.Neq{"moderation_case.resolution": moderation_model.ResolutionMerged})).
		Desc("moderation_case.issue_id").
		Cols("moderation_case.issue_id").
		Get(&id)
	if err != nil {
		return 0, fmt.Errorf("find previous case: %w", err)
	}
	return id, nil
}

// isRepeat applies the duplicate rule (foundations decisions 4) to one ticked reason: the
// reporter already reported it, still pending, on an overlapping anchor in its open case.
func isRepeat(ctx context.Context, caseIssueID int64, in ReportInput, reason moderation_model.Reason, a *resolvedAnchor) (bool, error) {
	earlier, err := moderation_model.FindPendingSubmissionsForReason(ctx, caseIssueID, in.Reporter.ID, reason)
	if err != nil {
		return false, fmt.Errorf("find earlier reports: %w", err)
	}
	return slices.ContainsFunc(earlier, func(s *moderation_model.Submission) bool { return anchorsOverlap(s, in.Anchor, a) }), nil
}

func anchorsOverlap(s *moderation_model.Submission, status moderation_model.AnchorStatus, a *resolvedAnchor) bool {
	if s.AnchorStatus == moderation_model.AnchorWhole || status == moderation_model.AnchorWhole {
		return s.AnchorStatus == status
	}
	if s.AnchorStatus == moderation_model.AnchorExact && status == moderation_model.AnchorExact && s.CommitSHA == a.commitSHA {
		return s.SourceStart < int64(a.end) && int64(a.start) < s.SourceEnd
	}
	// another revision, or a quote-only anchor: the same section and the same quote
	return s.SectionAnchor == a.section && s.Quote == a.quote
}

// createReasonIssue creates the reason issue of one ticked reason: poster the reporter,
// labels type/report and the reason, content the explanation.
func createReasonIssue(ctx context.Context, casesRepo *repo_model.Repository, in ReportInput, r ReasonInput, labels map[string]*issues_model.Label) (*issues_model.Issue, error) {
	issue := &issues_model.Issue{
		RepoID:   casesRepo.ID,
		Repo:     casesRepo,
		Title:    string(r.Reason) + ": " + in.Repo.Name,
		PosterID: in.Reporter.ID,
		Poster:   in.Reporter,
	}
	labelIDs := []int64{labels[moderation_model.LabelTypeReport].ID, labels[r.Reason.LabelName()].ID}
	// Created without content, which is set afterwards: issue creation adds cross-references
	// from the content, so a reporter could otherwise put a "referenced" comment, pointing at
	// a private moderation issue, on any issue they can read.
	if err := issues_model.NewIssue(ctx, casesRepo, issue, labelIDs, nil); err != nil {
		return nil, fmt.Errorf("create report issue: %w", err)
	}
	issue.Content = r.Explanation
	if err := issues_model.UpdateIssueCols(ctx, issue, "content"); err != nil {
		return nil, fmt.Errorf("set report content: %w", err)
	}
	return issue, nil
}

// syncCaseLabels writes the case's label projection (foundations decisions 1.2): type/case,
// its kind, source/reader and its reason. Any other reason label is removed.
func syncCaseLabels(ctx context.Context, issue *issues_model.Issue, c *moderation_model.Case, labels map[string]*issues_model.Label, doer *user_model.User) error {
	reasonLabel := c.Reason.LabelName()
	want := []*issues_model.Label{labels[moderation_model.LabelTypeCase], labels[c.Kind.LabelName()], labels[moderation_model.LabelSourceReader], labels[reasonLabel]}
	if err := issues_model.NewIssueLabels(ctx, issue, want, doer); err != nil {
		return fmt.Errorf("set case labels: %w", err)
	}
	for _, l := range issue.Labels {
		if strings.HasPrefix(l.Name, moderation_model.ReasonLabelPrefix) && l.Name != reasonLabel {
			if err := issues_model.DeleteIssueLabel(ctx, issue, l, doer); err != nil {
				return fmt.Errorf("remove case label %q: %w", l.Name, err)
			}
		}
	}
	return nil
}

// updateCasePriority recalculates the case's priority: child-safety cases first, then open
// Request sources cases whose deadline passed, then cases with three or more submissions.
// Reports on hold don't count: a child-safety case whose reports are all on hold is normal.
func updateCasePriority(ctx context.Context, issue *issues_model.Issue, c *moderation_model.Case) error {
	n, err := moderation_model.CountCaseSubmissions(ctx, issue.ID)
	if err != nil {
		return fmt.Errorf("count case submissions: %w", err)
	}
	priority := PriorityNormal
	switch {
	case c.Reason == moderation_model.ReasonChildSafety && n > 0:
		priority = PriorityChildSafety
	case deadlinePassed(issue, c, timeutil.TimeStampNow()):
		priority = PriorityDeadlinePassed
	case n >= highPrioritySubmits:
		priority = PriorityHigh
	}
	if issue.Priority == priority {
		return nil
	}
	issue.Priority = priority
	if err := issues_model.UpdateIssueCols(ctx, issue, "priority"); err != nil {
		return fmt.Errorf("set case priority: %w", err)
	}
	log.Trace("moderation case %d priority %d", issue.ID, priority)
	return nil
}

// deadlinePassed reports whether an open Request sources case is past its deadline.
func deadlinePassed(issue *issues_model.Issue, c *moderation_model.Case, now timeutil.TimeStamp) bool {
	return !issue.IsClosed && c.Decision == moderation_model.DecisionSourcesRequested &&
		issue.DeadlineUnix > 0 && issue.DeadlineUnix <= now
}
