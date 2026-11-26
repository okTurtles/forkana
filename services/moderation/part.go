// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"
	"fmt"
	"strconv"

	"code.gitea.io/gitea/models/db"
	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/globallock"
	issue_indexer "code.gitea.io/gitea/modules/indexer/issues"
	"code.gitea.io/gitea/modules/util"
)

// ChangePartResult is what ChangeCasePart did.
type ChangePartResult struct {
	// Case is the case the reports are in now: the moved case, or the one it was merged into.
	Case *moderation_model.Case
	// Merged is set when an open case for the same reason on the new part existed: the
	// changed case was merged into it and closed.
	Merged bool
}

// ChangeCasePart moves an open case reported on the wrong part of its article to another part
// (Change part, foundations decisions 13). Section is a heading id of the article's current
// README, moderation_model.SectionIntro or moderation_model.SectionWholeArticle. When the
// article already has an open case for the same reason on that part, the case is merged into
// it: its reports move there, and it closes with ResolutionMerged.
func ChangeCasePart(ctx context.Context, doer *user_model.User, caseIssueID int64, section string) (*ChangePartResult, error) {
	isModerator, err := IsModerator(ctx, doer)
	if err != nil {
		return nil, fmt.Errorf("check moderator: %w", err)
	}
	if !isModerator {
		return nil, util.NewPermissionDeniedErrorf("only moderators can change a case's part")
	}
	c, err := moderation_model.GetCase(ctx, caseIssueID)
	if err != nil {
		return nil, fmt.Errorf("get case %d: %w", caseIssueID, err)
	}
	if c == nil || c.TargetRepoID == 0 {
		return nil, util.NewNotExistErrorf("case %d isn't a case on an article", caseIssueID)
	}
	repo, err := repo_model.GetRepositoryByID(ctx, c.TargetRepoID)
	if err != nil {
		return nil, fmt.Errorf("get case article: %w", err)
	}
	title, err := partTitle(ctx, repo, section)
	if err != nil {
		return nil, err
	}

	// The same lock as SubmitReport: no report files into either case while it moves.
	release, err := globallock.Lock(ctx, fmt.Sprintf("moderation:case:repo:%d", c.TargetRepoID))
	if err != nil {
		return nil, fmt.Errorf("lock the article's cases: %w", err)
	}
	defer release()

	role := moderation_model.ActorModerator
	if doer.IsAdmin {
		role = moderation_model.ActorAdmin
	}
	var result *ChangePartResult
	var touched []int64
	err = db.WithTx(ctx, func(ctx context.Context) error {
		result, touched = nil, nil
		c, err := moderation_model.GetCase(ctx, caseIssueID) // again, under the lock
		if err != nil {
			return fmt.Errorf("get case %d: %w", caseIssueID, err)
		}
		issue, err := issues_model.GetIssueByID(ctx, c.IssueID)
		if err != nil {
			return fmt.Errorf("get case issue: %w", err)
		}
		if issue.IsClosed {
			return util.NewInvalidArgumentErrorf("case %d is closed", c.IssueID)
		}
		if c.SectionAnchor == section {
			result = &ChangePartResult{Case: c}
			return nil
		}
		into, err := moderation_model.FindOpenCase(ctx, c.TargetRepoID, section, c.Reason)
		if err != nil {
			return fmt.Errorf("find the case on the new part: %w", err)
		}
		touched = append(touched, c.IssueID)
		if into == nil {
			return moveCase(ctx, doer, role, c, issue, repo, section, title, &result)
		}
		touched = append(touched, into.IssueID)
		return mergeCase(ctx, doer, role, c, issue, into, &result)
	})
	if err != nil {
		return nil, err
	}
	for _, id := range touched {
		issue_indexer.UpdateIssueIndexer(ctx, id)
	}
	return result, nil
}

func moveCase(ctx context.Context, doer *user_model.User, role moderation_model.ActorRole, c *moderation_model.Case, issue *issues_model.Issue, repo *repo_model.Repository, section, title string, result **ChangePartResult) error {
	from := c.SectionAnchor
	if err := moderation_model.SetCasePart(ctx, c.IssueID, section, title); err != nil {
		return fmt.Errorf("set case part: %w", err)
	}
	c.SectionAnchor, c.SectionTitle = section, title
	if err := repo.LoadOwner(ctx); err != nil {
		return fmt.Errorf("load article owner: %w", err)
	}
	if err := repo.LoadSubject(ctx); err != nil {
		return fmt.Errorf("load article subject: %w", err)
	}
	// Set directly rather than through ChangeIssueTitle, which adds cross-references.
	issue.Title = caseTitle(repo, section, title, c.Reason)
	if err := issues_model.UpdateIssueCols(ctx, issue, "name"); err != nil {
		return fmt.Errorf("set case title: %w", err)
	}
	if err := moderation_model.InsertAudit(ctx, &moderation_model.Audit{
		ActorID:      doer.ID,
		ActorRole:    role,
		Action:       moderation_model.AuditPartChanged,
		CaseIssueID:  c.IssueID,
		TargetUserID: c.TargetUserID,
		TargetRepoID: c.TargetRepoID,
		Reason:       c.Reason,
		Summary:      util.EllipsisDisplayString("from:"+from, moderation_model.MaxAuditSummaryLength),
	}); err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	*result = &ChangePartResult{Case: c}
	return nil
}

func mergeCase(ctx context.Context, doer *user_model.User, role moderation_model.ActorRole, c *moderation_model.Case, issue *issues_model.Issue, into *moderation_model.Case, result **ChangePartResult) error {
	intoIssue, err := issues_model.GetIssueByID(ctx, into.IssueID)
	if err != nil {
		return fmt.Errorf("get merged-into case issue: %w", err)
	}
	reports, err := moderation_model.FindReportsByCase(ctx, c.IssueID)
	if err != nil {
		return fmt.Errorf("find case reports: %w", err)
	}
	// The case dependencies follow the reports: the merged case can then close, and the case
	// it was merged into can't close before they are decided.
	for _, r := range reports {
		reportIssue, err := issues_model.GetIssueByID(ctx, r.IssueID)
		if err != nil {
			return fmt.Errorf("get report issue: %w", err)
		}
		if err := issues_model.RemoveIssueDependency(ctx, doer, issue, reportIssue, issues_model.DependencyTypeBlockedBy); err != nil && !issues_model.IsErrDependencyNotExists(err) {
			return fmt.Errorf("unlink report from case: %w", err)
		}
		if err := issues_model.CreateIssueDependency(ctx, doer, intoIssue, reportIssue); err != nil && !issues_model.IsErrDependencyExists(err) {
			return fmt.Errorf("link report to case: %w", err)
		}
	}
	if err := moderation_model.MoveReports(ctx, c.IssueID, into.IssueID); err != nil {
		return fmt.Errorf("move reports: %w", err)
	}
	if err := moderation_model.MarkMerged(ctx, c.IssueID, into.IssueID); err != nil {
		return fmt.Errorf("mark case merged: %w", err)
	}
	if _, err := issues_model.CloseIssue(ctx, issue, doer); err != nil {
		return fmt.Errorf("close merged case: %w", err)
	}
	if err := updateCasePriority(ctx, intoIssue, into); err != nil {
		return err
	}
	if err := moderation_model.InsertAudit(ctx, &moderation_model.Audit{
		ActorID:      doer.ID,
		ActorRole:    role,
		Action:       moderation_model.AuditMerged,
		CaseIssueID:  c.IssueID,
		TargetUserID: c.TargetUserID,
		TargetRepoID: c.TargetRepoID,
		Reason:       c.Reason,
		Summary:      "into:" + strconv.FormatInt(into.IssueID, 10),
	}); err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	*result = &ChangePartResult{Case: into, Merged: true}
	return nil
}

// partTitle checks a part of the article and returns its title: "" for the whole article and
// the introduction, or the text of the heading with the id in the current README.
func partTitle(ctx context.Context, repo *repo_model.Repository, section string) (string, error) {
	if section == moderation_model.SectionWholeArticle || section == moderation_model.SectionIntro {
		return "", nil
	}
	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("open repository: %w", err)
	}
	defer gitRepo.Close()
	commit, err := gitRepo.GetBranchCommit(repo.DefaultBranch)
	if err != nil {
		return "", fmt.Errorf("get the article's %s: %w", repo.DefaultBranch, err)
	}
	raw, err := readArticleAt(ctx, repo, commit.ID.String())
	if err != nil {
		return "", err
	}
	title := headingTitle(ctx, raw, section)
	if title == "" {
		return "", invalid("the article has no section %q", section)
	}
	return title, nil
}
