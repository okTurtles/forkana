// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"time"

	git_model "code.gitea.io/gitea/models/git"
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

// articleContributorCountCacheKey is the cache key of the count of repo whose default
// branch was at head, counted since `since`. The head only detects a change of the
// branch: the count itself is always made on the real branch tip (see
// countArticleContributors). "v4": v1 counted on the head itself, which gave wrong
// counts for a stale head (see nodeContributorStats), and v3 counted author emails
// for a while; neither is reused. (v2 had the same counts as v4.)
func articleContributorCountCacheKey(repoID int64, head string, since time.Time) string {
	return fmt.Sprintf("ArticleContributorCount/v4/%d/%s/%d", repoID, head, since.Unix())
}

// cachedArticleContributorCount returns the cached count of repo at head, if any.
func cachedArticleContributorCount(repo *repo_model.Repository, head string) (int64, bool) {
	c := cache.GetCache()
	if c == nil || head == "" {
		return 0, false
	}
	var cached int64
	exist, err := c.GetJSON(articleContributorCountCacheKey(repo.ID, head, ArticleContributorSince(repo)), &cached)
	return cached, exist && err == nil
}

// countArticleContributors counts the contributors of repo on its default branch, as
// git has it now, and caches the count under head.
//
// The count is made on the branch, never on head: head may come from the database
// (nodeContributorStats), whose branch table is updated asynchronously after a push
// and can lag behind, or be stale for good in data written straight into the
// repositories. Counting on a stale head gives the count of an old commit (0 for a
// fork whose head is still its fork point). head only keys the cache. The price is a
// small race: a push between reading head and counting stores the newer count under
// the older head, which the next push's new head replaces.
func countArticleContributors(gitRepo *git.Repository, repo *repo_model.Repository, head string) (int64, error) {
	since := ArticleContributorSince(repo)
	// git shortlog groups the commits by author NAME, on purpose: the count is the
	// number of people the article credits, as its history shows them.
	count, err := gitRepo.GetContributorCount(repo.DefaultBranch, since)
	if err != nil {
		return 0, err
	}
	if c := cache.GetCache(); c != nil {
		if err := c.PutJSON(articleContributorCountCacheKey(repo.ID, head, since), count, articleContributorCountCacheTTL); err != nil {
			log.Warn("Failed to cache the contributor count of %s: %v", repo.FullName(), err)
		}
	}
	return count, nil
}

// ArticleContributorCountWithGitRepo returns the number of contributors of an article:
// the distinct authors on its default branch since ArticleContributorSince. It is the
// one count every view of the subject shows (the bubbles, the table, the article view
// and the compare page), so they can never disagree (#405).
func ArticleContributorCountWithGitRepo(gitRepo *git.Repository, repo *repo_model.Repository) (int64, error) {
	if repo.IsEmpty {
		return 0, nil
	}
	head, err := gitRepo.GetBranchCommitID(repo.DefaultBranch)
	if err != nil {
		return 0, err
	}
	if count, ok := cachedArticleContributorCount(repo, head); ok {
		return count, nil
	}
	return countArticleContributors(gitRepo, repo, head)
}

// ArticleContributorCountOrUnknown is ArticleContributorCountWithGitRepo for a view: -1
// when the count cannot be computed, which the views show as unknown ("-").
func ArticleContributorCountOrUnknown(gitRepo *git.Repository, repo *repo_model.Repository) int64 {
	count, err := ArticleContributorCountWithGitRepo(gitRepo, repo)
	if err != nil {
		log.Warn("Failed to get contributor count for %s: %v", repo.FullName(), err)
		return -1
	}
	return count
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

// branchHeads reads the head commit of each repository's default branch from the
// database, in one query. A repository missing from the result has no recorded head.
func branchHeads(ctx context.Context, repos []*repo_model.Repository) map[int64]string {
	want := make(map[int64]string, len(repos))
	for _, repo := range repos {
		if !repo.IsEmpty && repo.DefaultBranch != "" {
			want[repo.ID] = repo.DefaultBranch
		}
	}
	if len(want) == 0 {
		return nil
	}
	heads, err := git_model.FindBranchesByRepoAndBranchName(ctx, want)
	if err != nil {
		log.Warn("Failed to read the branch heads of the fork graph: %v", err)
		return nil
	}
	return heads
}

// nodeContributorStats returns the contributor stats of one fork graph node: the
// article's contributor count exactly as every other view of the subject shows it, so a
// bubble, its table row and its article page always carry the same number (#405).
// RecentCount is not computed (see ContributorStats). Returns nil when the count cannot
// be computed, which the client shows as unknown.
//
// head is the branch head recorded in the database (empty if there is no branch row):
// with it, a cached count is answered without opening the repository. It is only a
// cache key: a count is always made on the real branch tip (countArticleContributors),
// and without it the head is read from git. A count gets its own budget
// (nodeContributorCountTimeout) inside ctx, which carries the whole counting phase's
// deadline and the request's cancellation: once that is spent, no count starts, and
// the node is unknown.
func nodeContributorStats(ctx context.Context, repo *repo_model.Repository, head string) *ContributorStats {
	if repo.IsEmpty {
		return &ContributorStats{TotalCount: 0}
	}
	if count, ok := cachedArticleContributorCount(repo, head); ok {
		return &ContributorStats{TotalCount: int(count)}
	}
	if ctx.Err() != nil {
		return nil // the counting phase is over (deadline or client gone): unknown
	}
	countCtx, cancel := context.WithTimeout(ctx, nodeContributorCountTimeout)
	defer cancel()
	total, err := func() (int64, error) {
		gitRepo, err := gitrepo.OpenRepository(countCtx, repo)
		if err != nil {
			return 0, err
		}
		defer gitRepo.Close()
		if head == "" {
			return ArticleContributorCountWithGitRepo(gitRepo, repo)
		}
		return countArticleContributors(gitRepo, repo, head)
	}()
	if err != nil {
		log.Warn("Failed to get contributor count for %s: %v", repo.FullName(), err)
		return nil
	}
	return &ContributorStats{TotalCount: int(total)}
}
