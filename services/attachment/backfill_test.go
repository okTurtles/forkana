// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitTry(t *testing.T, dir, stdin string, args ...string) (string, error) {
	t.Helper()
	var stdout strings.Builder
	var stderr strings.Builder
	cmd := gitcmd.NewCommand().AddArguments(gitcmd.ToTrustedCmdArgs(args)...)
	err := cmd.Run(t.Context(), &gitcmd.RunOpts{
		Dir:    dir,
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		Env: append(os.Environ(),
			"GIT_AUTHOR_NAME=tester", "GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=tester", "GIT_COMMITTER_EMAIL=tester@example.com",
		),
	})
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func gitRun(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	out, err := gitTry(t, dir, stdin, args...)
	require.NoError(t, err)
	return out
}

// commitArticle adds one revision of the article content, plus a companion file
// outside the article paths, so a scan has something it must ignore.
func commitArticle(t *testing.T, repoPath, article, other string) {
	t.Helper()
	tree := fmt.Sprintf("100644 blob %s\tREADME.md\n", gitRun(t, repoPath, article, "hash-object", "-w", "--stdin"))
	if other != "" {
		tree += fmt.Sprintf("100644 blob %s\tnotes.md\n", gitRun(t, repoPath, other, "hash-object", "-w", "--stdin"))
	}
	treeSHA := gitRun(t, repoPath, tree, "mktree")

	args := []string{"commit-tree", treeSHA}
	// The first revision has no parent, which rev-parse reports by failing.
	if parent, err := gitTry(t, repoPath, "", "rev-parse", "--verify", "--quiet", "refs/heads/master^{commit}"); err == nil && parent != "" {
		args = append(args, "-p", parent)
	}
	commitSHA := gitRun(t, repoPath, "article", args...)
	gitRun(t, repoPath, "", "update-ref", "refs/heads/master", commitSHA)
}

// newArticleRepo creates a repository whose git directory this package may
// write to, rather than mutating the shared fixture repositories.
func newArticleRepo(t *testing.T, name string) *repo_model.Repository {
	t.Helper()
	repo := &repo_model.Repository{
		OwnerID:       2,
		OwnerName:     "user2",
		Name:          name,
		LowerName:     strings.ToLower(name),
		DefaultBranch: "master",
	}
	require.NoError(t, db.Insert(t.Context(), repo))
	require.NoError(t, os.MkdirAll(repo.RepoPath(), 0o755))
	gitRun(t, repo.RepoPath(), "", "init", "--bare", "--initial-branch=master")
	t.Cleanup(func() {
		_, _ = db.GetEngine(t.Context()).ID(repo.ID).Delete(&repo_model.Repository{})
		_ = os.RemoveAll(repo.RepoPath())
	})
	return repo
}

func newLegacyAttachment(t *testing.T, uuid string, repoID int64) *repo_model.Attachment {
	t.Helper()
	attach := &repo_model.Attachment{
		UUID:       uuid,
		RepoID:     repoID,
		UploaderID: 2,
		Purpose:    repo_model.AttachmentPurposeUnspecified,
		Name:       "image.png",
	}
	require.NoError(t, db.Insert(t.Context(), attach))
	return attach
}

func TestScanArticleHistoryUUIDs(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	const (
		firstUUID   = "7f2c9a31-0000-4000-8000-0000000b0001"
		secondUUID  = "7f2c9a31-0000-4000-8000-0000000b0002"
		ignoredUUID = "7f2c9a31-0000-4000-8000-0000000b0003"
	)
	repo := newArticleRepo(t, "scan-history")
	commitArticle(t, repo.RepoPath(), articleContent(firstUUID), "")
	commitArticle(t, repo.RepoPath(), articleContent(secondUUID), articleContent(ignoredUUID))

	uuids, err := scanArticleHistoryUUIDs(t.Context(), repo.RepoPath())
	require.NoError(t, err)

	// the reference only an overwritten revision carries still counts: an
	// association outlives the content that introduced it
	assert.ElementsMatch(t, []string{firstUUID, secondUUID}, uuids)
}

func TestBackfillArticleAttachments(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	const (
		legacyUUID  = "7f2c9a31-0000-4000-8000-0000000c0001"
		foreignUUID = "7f2c9a31-0000-4000-8000-0000000c0002"
		goneUUID    = "7f2c9a31-0000-4000-8000-0000000c0003"
	)
	repo := newArticleRepo(t, "backfill-article")
	legacy := newLegacyAttachment(t, legacyUUID, repo.ID)
	foreign := newLegacyAttachment(t, foreignUUID, 3)
	commitArticle(t, repo.RepoPath(), articleContent(legacyUUID, foreignUUID, goneUUID), "")

	opts := BackfillOptions{StartRepoID: repo.ID, DryRun: true}
	result, err := BackfillArticleAttachments(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, 3, result.ReferencesFound)
	assert.Equal(t, 1, result.Outstanding)
	assert.Equal(t, 1, result.SuspiciousSkipped)
	assert.Equal(t, 1, result.MissingAttachments)
	assert.Equal(t, 0, result.AssociationsInserted)
	unittest.AssertNotExistsBean(t, &repo_model.ArticleAttachment{RepoID: repo.ID, AttachmentID: legacy.ID})

	opts.DryRun = false
	result, err = BackfillArticleAttachments(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, 1, result.AssociationsInserted)
	assert.Equal(t, 1, result.LegacyInferred)
	assert.Equal(t, 1, result.SuspiciousSkipped)

	// the inferred row becomes a first-class article attachment, while the
	// attachment of an unrelated repository is only reported
	unittest.AssertExistsAndLoadBean(t, &repo_model.ArticleAttachment{RepoID: repo.ID, AttachmentID: legacy.ID})
	unittest.AssertNotExistsBean(t, &repo_model.ArticleAttachment{RepoID: repo.ID, AttachmentID: foreign.ID})
	reloaded := unittest.AssertExistsAndLoadBean(t, &repo_model.Attachment{ID: legacy.ID})
	assert.Equal(t, repo_model.AttachmentPurposeArticle, reloaded.Purpose)

	// running it again is a no-op, and a resume past the repository skips it
	result, err = BackfillArticleAttachments(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, 0, result.AssociationsInserted)
	assert.Equal(t, 1, result.ReposScanned)

	opts.StartRepoID = repo.ID + 1
	result, err = BackfillArticleAttachments(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ReposScanned)
}

func TestFinalizeLegacyFallback(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	fallbackKey := setting.Config().Attachment.LegacyArticleFallback.DynKey()
	t.Cleanup(func() {
		require.NoError(t, system_model.SetSettings(t.Context(), map[string]string{fallbackKey: "true"}))
	})
	// The stored setting is what finalizing writes; the in-process value is
	// only refreshed when its cache expires.
	storedFallback := func(t *testing.T) string {
		t.Helper()
		_, settings, err := system_model.GetAllSettings(t.Context())
		require.NoError(t, err)
		return settings[fallbackKey]
	}

	const legacyUUID = "7f2c9a31-0000-4000-8000-0000000d0001"
	repo := newArticleRepo(t, "finalize-article")
	newLegacyAttachment(t, legacyUUID, repo.ID)
	commitArticle(t, repo.RepoPath(), articleContent(legacyUUID), "")

	// an outstanding reference would 404 the moment the fallback is switched off
	_, err := FinalizeLegacyFallback(t.Context(), BackfillOptions{})
	require.ErrorContains(t, err, "not associated yet")
	assert.NotEqual(t, "false", storedFallback(t))

	_, err = BackfillArticleAttachments(t.Context(), BackfillOptions{StartRepoID: repo.ID})
	require.NoError(t, err)

	result, err := FinalizeLegacyFallback(t.Context(), BackfillOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Outstanding)
	assert.Equal(t, "false", storedFallback(t))
}
