// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"time"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/cache"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/log"
)

// articleContributorCountCacheTTL is how long a count is cached. A count is immutable:
// the key carries the branch head (and the "since" time), so a push makes a new key
// rather than serving a stale count. The TTL therefore only has to bound how many keys
// pile up (one per push, not per view); a short one would just make the first visitor
// after each expiry recount every article of the subject.
const articleContributorCountCacheTTL int64 = 7 * 24 * 60 * 60

// nodeContributorCountTimeout is the budget of one fork graph node's count.
const nodeContributorCountTimeout = 5 * time.Second

// ArticleContributorSince returns the time from which commits count towards an
// article's contributors. A fork only counts the contributors who committed after it
// was created, so the inherited history of its parent is not credited to it; any other
// article counts its whole history (zero time).
func ArticleContributorSince(repo *repo_model.Repository) time.Time {
	if repo.IsFork && repo.CreatedUnix > 0 {
		return repo.CreatedUnix.AsTime()
	}
	return time.Time{}
}

// ArticleContributorCountWithGitRepo returns the number of contributors of an article:
// the distinct authors on its default branch since ArticleContributorSince. It is the
// one count every view of the subject shows (the bubbles, the table, the article view
// and the compare page), so they can never disagree (#405).
func ArticleContributorCountWithGitRepo(gitRepo *git.Repository, repo *repo_model.Repository) (int64, error) {
	if repo.IsEmpty {
		return 0, nil
	}
	branch := repo.DefaultBranch
	since := ArticleContributorSince(repo)

	c := cache.GetCache()
	cacheKey := ""
	if c != nil {
		if head, err := gitRepo.GetBranchCommitID(branch); err == nil {
			cacheKey = fmt.Sprintf("ArticleContributorCount/%d/%s/%d", repo.ID, head, since.Unix())
			var cached int64
			if exist, getErr := c.GetJSON(cacheKey, &cached); exist && getErr == nil {
				return cached, nil
			}
		}
	}

	count, err := gitRepo.GetContributorCount(branch, since)
	if err != nil {
		return 0, err
	}
	if cacheKey != "" {
		if err := c.PutJSON(cacheKey, count, articleContributorCountCacheTTL); err != nil {
			log.Warn("Failed to cache the contributor count of %s: %v", repo.FullName(), err)
		}
	}
	return count, nil
}

// nodeContributorStats returns the contributor stats of one fork graph node: the
// article's contributor count exactly as every other view of the subject shows it, so a
// bubble, its table row and its article page always carry the same number (#405).
// RecentCount is not computed (see ContributorStats). Returns nil when the count cannot
// be computed, which the client shows as unknown.
//
// Each count has a budget of its own (nodeContributorCountTimeout), detached from the
// request's cancellation (context.WithoutCancel) so that it cannot be cut short by
// anything but that budget: a count is best effort, and a slow one shows up as "unknown"
// for that node instead of holding the page or failing the whole graph.
func nodeContributorStats(ctx context.Context, repo *repo_model.Repository) *ContributorStats {
	countCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nodeContributorCountTimeout)
	defer cancel()
	total, err := ArticleContributorCount(countCtx, repo)
	if err != nil {
		log.Warn("Failed to get contributor count for %s: %v", repo.FullName(), err)
		return nil
	}
	return &ContributorStats{TotalCount: int(total)}
}

// ArticleContributorCount is ArticleContributorCountWithGitRepo for a caller that has
// no git repository open.
func ArticleContributorCount(ctx context.Context, repo *repo_model.Repository) (int64, error) {
	if repo.IsEmpty {
		return 0, nil
	}
	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return 0, err
	}
	defer gitRepo.Close()
	return ArticleContributorCountWithGitRepo(gitRepo, repo)
}
