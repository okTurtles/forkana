// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"context"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
)

// gitZeroTime is how git reads Go's zero time, "0001-01-01T00:00:00Z": the date the
// contents API gave every commit it made without "dates" until #478 fixed it
// (routers/api/v1/repo/file.go). The first article-creator sent none, so the import
// commit of those articles is dated 2001-01-01T00:00:00Z.
var gitZeroTime = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// articleChangeTimes tells when the changes of an article were made, as its History tab
// and its "Last updated" token say it: the commit's date, except for a commit carrying
// gitZeroTime, whose real date is lost. That one came with the article's import, so it is
// dated by the creation of the article it was imported into: the root of the fork network
// (a fork copies its parent's commits, and is created later).
type articleChangeTimes struct {
	ctx      context.Context
	repo     *repo_model.Repository
	imported time.Time // the root article's creation, once looked up
}

func newArticleChangeTimes(ctx context.Context, repo *repo_model.Repository) *articleChangeTimes {
	return &articleChangeTimes{ctx: ctx, repo: repo}
}

// of returns when the commit was made (its committer's date, else its author's).
func (t *articleChangeTimes) of(commit *git.Commit) time.Time {
	sig := commit.Committer
	if sig == nil {
		sig = commit.Author
	}
	if sig == nil {
		return time.Time{}
	}
	if !sig.When.Equal(gitZeroTime) {
		return sig.When
	}
	if t.imported.IsZero() {
		t.imported = articleRootCreated(t.ctx, t.repo)
	}
	if t.imported.IsZero() {
		return sig.When
	}
	return t.imported
}

// articleRootCreated is when the root of the article's fork network was created: one
// query finds the root (FindForkTreeRoot's recursive CTE), one loads it. A fork whose
// parent is gone counts as its own root, the topmost repository the CTE can reach.
func articleRootCreated(ctx context.Context, repo *repo_model.Repository) time.Time {
	root := repo
	if repo.IsFork && repo.ForkID > 0 {
		rootID, err := repo_model.FindForkTreeRoot(ctx, repo.ID)
		switch {
		case err != nil:
			log.Warn("FindForkTreeRoot(%d): %v", repo.ID, err)
		case rootID != repo.ID:
			if r, err := repo_model.GetRepositoryByID(ctx, rootID); err == nil {
				root = r
			} else if !repo_model.IsErrRepoNotExist(err) {
				log.Warn("GetRepositoryByID(%d): %v", rootID, err)
			}
		}
	}
	if root.CreatedUnix == 0 {
		return time.Time{}
	}
	return root.CreatedUnix.AsTime()
}
