// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"
	"fmt"
	"html/template"

	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/models/renderhelper"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/markup/markdown"
)

// CasePassage is a case's article rendered at the revision of a reported passage, with every
// block carrying its source range and Range the block to highlight.
type CasePassage struct {
	HTML      template.HTML
	Range     string
	CommitSHA string
}

// RenderCasePassage renders the article of a case's first exact submission at the reported
// revision, or returns nil when the case has none. Only moderators reach it: the case page
// checks access, and the article is shown as it was when it was reported.
func RenderCasePassage(ctx context.Context, c *moderation_model.Case) (*CasePassage, error) {
	if c == nil || c.TargetRepoID == 0 {
		return nil, nil
	}
	subs, err := moderation_model.FindSubmissionsByCase(ctx, c.IssueID)
	if err != nil {
		return nil, fmt.Errorf("find case submissions: %w", err)
	}
	var sub *moderation_model.Submission
	for _, s := range subs {
		if s.AnchorStatus == moderation_model.AnchorExact {
			sub = s
			break
		}
	}
	if sub == nil {
		return nil, nil
	}
	repo, err := repo_model.GetRepositoryByID(ctx, c.TargetRepoID)
	if err != nil {
		return nil, fmt.Errorf("get case article: %w", err)
	}
	raw, err := readArticleAt(ctx, repo, sub.CommitSHA)
	if err != nil {
		return nil, err
	}
	rctx := renderhelper.NewRenderContextRepoFile(ctx, repo, renderhelper.RepoFileOptions{
		CurrentRefPath:  "commit/" + sub.CommitSHA,
		CurrentTreePath: "",
	}).WithSourceRanges(true)
	html, err := markdown.RenderString(rctx, string(raw))
	if err != nil {
		return nil, fmt.Errorf("render case article: %w", err)
	}
	return &CasePassage{
		HTML:      html,
		Range:     fmt.Sprintf("%d-%d", sub.SourceStart, sub.SourceEnd),
		CommitSHA: sub.CommitSHA,
	}, nil
}
