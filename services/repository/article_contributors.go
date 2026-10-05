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

// articleContributorCountCacheTTL is how long a count is cached. The key carries the
// branch head, so a push makes a new key rather than serving a stale count; the TTL
// only bounds how long unused keys linger.
const articleContributorCountCacheTTL int64 = 60 * 60

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
func nodeContributorStats(ctx context.Context, repo *repo_model.Repository) *ContributorStats {
	total, err := ArticleContributorCount(ctx, repo)
	if err != nil {
		log.Warn("Failed to get contributor count for repo %d: %v", repo.ID, err)
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
