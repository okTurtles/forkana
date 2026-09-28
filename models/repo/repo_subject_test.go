// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"context"
	"strings"
	"testing"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/cache"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
)

func TestGetPublicRepositoryBySubject(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Test Subject")
	assert.NoError(t, err)
	assert.NotNil(t, subject)

	// Get a repository and assign it the subject
	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	assert.NotNil(t, repo)

	repo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id")
	assert.NoError(t, err)

	// Test GetPublicRepositoryBySubject
	foundRepo, err := repo_model.GetPublicRepositoryBySubject(ctx, "Test Subject")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, repo.ID, foundRepo.ID)
	assert.NotNil(t, foundRepo.SubjectRelation)
	assert.Equal(t, subject.ID, foundRepo.SubjectRelation.ID)
	assert.Equal(t, "Test Subject", foundRepo.SubjectRelation.Name)
}

func TestGetPublicRepositoryBySubject_NotFound(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Try to get a repository with a non-existent subject
	_, err := repo_model.GetPublicRepositoryBySubject(ctx, "Non-Existent Subject")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrSubjectNotExist(err))
}

func TestGetPublicRepositoryBySubject_NoPublicRepo(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a subject without any public repository
	subject, err := repo_model.GetOrCreateSubject(ctx, "Subject Without Public Repo")
	assert.NoError(t, err)
	assert.NotNil(t, subject)

	// Try to get a public repository for this subject
	_, err = repo_model.GetPublicRepositoryBySubject(ctx, "Subject Without Public Repo")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrRepoWithSubjectNotExist(err))
}

func TestGetPublicRepositoryBySubject_PrefersRootRepo(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Shared Subject")
	assert.NoError(t, err)

	// Get two repositories - one root and one fork
	rootRepo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	rootRepo.IsFork = false
	rootRepo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, rootRepo, "subject_id", "is_fork")
	assert.NoError(t, err)

	forkRepo, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	forkRepo.IsFork = true
	forkRepo.ForkID = rootRepo.ID
	forkRepo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, forkRepo, "subject_id", "is_fork", "fork_id")
	assert.NoError(t, err)

	// GetPublicRepositoryBySubject should return the root repo, not the fork
	foundRepo, err := repo_model.GetPublicRepositoryBySubject(ctx, "Shared Subject")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, rootRepo.ID, foundRepo.ID)
	assert.False(t, foundRepo.IsFork)
}

func TestGetRepositoryByOwnerAndSubject(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Owner Subject Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo.LoadOwner(ctx)
	assert.NoError(t, err)

	repo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id")
	assert.NoError(t, err)

	// Test GetRepositoryByOwnerAndSubject
	foundRepo, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, repo.Owner.Name, "Owner Subject Test")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, repo.ID, foundRepo.ID)
	assert.NotNil(t, foundRepo.SubjectRelation)
	assert.Equal(t, subject.ID, foundRepo.SubjectRelation.ID)
}

func TestGetRepositoryByOwnerAndSubject_NotFound(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Try to get a repository with a non-existent subject
	_, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, "user1", "Non-Existent Subject")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrSubjectNotExist(err))
}

func TestGetRepositoryByOwnerAndSubject_WrongOwner(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Wrong Owner Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo.LoadOwner(ctx)
	assert.NoError(t, err)

	repo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id")
	assert.NoError(t, err)

	// Try to get the repository with a different owner
	_, err = repo_model.GetRepositoryByOwnerAndSubject(ctx, "different-user", "Wrong Owner Test")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrRepoNotExist(err))
}

func TestGetRepositoryByOwnerAndSubject_ReturnsCorrectRepo(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Multi Owner Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo1, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo1.LoadOwner(ctx)
	assert.NoError(t, err)
	repo1.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "subject_id")
	assert.NoError(t, err)

	// GetRepositoryByOwnerAndSubject should return the correct repository for the owner
	foundRepo, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, repo1.Owner.Name, "Multi Owner Test")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, repo1.ID, foundRepo.ID)
	assert.Equal(t, repo1.Owner.Name, foundRepo.OwnerName)
}

func TestGetRepositoryByOwnerAndSubject_PrefersActiveOverArchived(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Vanity URL Archived Test")
	assert.NoError(t, err)

	// The archived repository is the more recently updated one, so it would win a
	// plain "updated_unix DESC" ordering
	archivedRepo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = archivedRepo.LoadOwner(ctx)
	assert.NoError(t, err)
	archivedRepo.SubjectID = subject.ID
	archivedRepo.IsArchived = true
	archivedRepo.UpdatedUnix = timeutil.TimeStamp(2000)
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, archivedRepo, "subject_id", "is_archived", "updated_unix")
	assert.NoError(t, err)

	activeRepo, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, archivedRepo.OwnerID, activeRepo.OwnerID, "both repositories must share an owner")
	activeRepo.SubjectID = subject.ID
	activeRepo.UpdatedUnix = timeutil.TimeStamp(1000)
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, activeRepo, "subject_id", "updated_unix")
	assert.NoError(t, err)

	// The vanity URL points at the active article
	foundRepo, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, archivedRepo.Owner.Name, "Vanity URL Archived Test")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, activeRepo.ID, foundRepo.ID)
	assert.False(t, foundRepo.IsArchived)

	// Without an active article, the archived one is served as a fallback
	activeRepo.SubjectID = 0
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, activeRepo, "subject_id")
	assert.NoError(t, err)

	foundRepo, err = repo_model.GetRepositoryByOwnerAndSubject(ctx, archivedRepo.Owner.Name, "Vanity URL Archived Test")
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, archivedRepo.ID, foundRepo.ID)
	assert.True(t, foundRepo.IsArchived)

	// With no repository left for the subject, the lookup fails
	archivedRepo.SubjectID = 0
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, archivedRepo, "subject_id")
	assert.NoError(t, err)

	_, err = repo_model.GetRepositoryByOwnerAndSubject(ctx, archivedRepo.Owner.Name, "Vanity URL Archived Test")
	assert.Error(t, err)
	assert.True(t, repo_model.IsErrRepoNotExist(err))
}

func TestGetActiveRepositoryByOwnerIDAndSubjectID(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Active Getter Test")
	assert.NoError(t, err)

	// Two repositories of the same owner on the subject, the lower ID one archived
	archivedRepo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	archivedRepo.SubjectID = subject.ID
	archivedRepo.IsArchived = true
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, archivedRepo, "subject_id", "is_archived")
	assert.NoError(t, err)

	activeRepo, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, archivedRepo.OwnerID, activeRepo.OwnerID, "both repositories must share an owner")
	assert.Less(t, archivedRepo.ID, activeRepo.ID, "the archived repository must have the lower ID")
	activeRepo.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, activeRepo, "subject_id")
	assert.NoError(t, err)

	// The active repository wins even though the archived one has the lower ID
	foundRepo, err := repo_model.GetActiveRepositoryByOwnerIDAndSubjectID(ctx, activeRepo.OwnerID, subject.ID)
	assert.NoError(t, err)
	assert.NotNil(t, foundRepo)
	assert.Equal(t, activeRepo.ID, foundRepo.ID)
	assert.False(t, foundRepo.IsArchived)

	// With only the archived repository left, the owner counts as having none
	activeRepo.SubjectID = 0
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, activeRepo, "subject_id")
	assert.NoError(t, err)

	foundRepo, err = repo_model.GetActiveRepositoryByOwnerIDAndSubjectID(ctx, archivedRepo.OwnerID, subject.ID)
	assert.NoError(t, err)
	assert.Nil(t, foundRepo)
}

func TestGetActiveRepositoryByOwnerIDAndSubjectID_NoRepository(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Unused Subject Test")
	assert.NoError(t, err)

	// No repository for this subject: nil is returned without an error
	foundRepo, err := repo_model.GetActiveRepositoryByOwnerIDAndSubjectID(ctx, 2, subject.ID)
	assert.NoError(t, err)
	assert.Nil(t, foundRepo)
}

func TestGetRepositoriesBySubjectIDAndOwners(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Batch Query Test")
	assert.NoError(t, err)

	// Get two repositories and assign them the same subject
	repo1, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo1.LoadOwner(ctx)
	assert.NoError(t, err)
	repo1.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "subject_id")
	assert.NoError(t, err)

	repo2, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	err = repo2.LoadOwner(ctx)
	assert.NoError(t, err)
	repo2.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo2, "subject_id")
	assert.NoError(t, err)

	// Test fetching both repositories in a single query
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{repo1.Owner.Name, repo2.Owner.Name})
	assert.NoError(t, err)
	assert.Len(t, repos, 2)

	// Verify both repos are returned
	foundRepo1 := false
	foundRepo2 := false
	for _, r := range repos {
		if r.ID == repo1.ID {
			foundRepo1 = true
		}
		if r.ID == repo2.ID {
			foundRepo2 = true
		}
	}
	assert.True(t, foundRepo1, "repo1 should be in results")
	assert.True(t, foundRepo2, "repo2 should be in results")
}

func TestGetRepositoriesBySubjectIDAndOwners_CaseInsensitive(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Case Insensitive Test")
	assert.NoError(t, err)

	// Get a repository and assign it the subject
	repo1, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	err = repo1.LoadOwner(ctx)
	assert.NoError(t, err)
	repo1.SubjectID = subject.ID
	err = repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo1, "subject_id")
	assert.NoError(t, err)

	// Test with different case variations
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{
		"USER2", // uppercase version of owner name
	})
	assert.NoError(t, err)
	// Should find the repo regardless of case
	assert.Len(t, repos, 1, "Should find repo regardless of case")
	assert.Equal(t, repo1.ID, repos[0].ID)
}

func TestGetRepositoriesBySubjectIDAndOwners_NoMatches(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "No Matches Test")
	assert.NoError(t, err)

	// Query with non-existent owners
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{"nonexistent1", "nonexistent2"})
	assert.NoError(t, err)
	assert.Empty(t, repos)
}

func TestGetRepositoriesBySubjectIDAndOwners_EmptyOwnerList(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	// Create a test subject
	subject, err := repo_model.GetOrCreateSubject(ctx, "Empty Owner List Test")
	assert.NoError(t, err)

	// Query with empty owner list
	repos, err := repo_model.GetRepositoriesBySubjectIDAndOwners(ctx, subject.ID, []string{})
	assert.NoError(t, err)
	assert.Empty(t, repos)
}

func TestRepositoryLinkArchivedUsesArticleIndex(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Link Routing Subject")
	assert.NoError(t, err)

	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	repo.SubjectID = subject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id"))

	base := setting.AppSubURL + "/subject/Link%20Routing%20Subject/" + repo.OwnerName

	// the owner's only article for the subject carries no index
	assert.Equal(t, base, repo.Link())

	// archiving it while the same owner holds an active article for the subject moves
	// it past the first position, which the url spells out
	other, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, repo.OwnerID, other.OwnerID)
	other.SubjectID = subject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, other, "subject_id"))

	assert.NoError(t, repo_model.SetArchiveRepoState(ctx, repo, true))
	assert.Equal(t, base+"/2", repo.Link())
	assert.Equal(t, 2, repo.ArticleIndex(ctx))
	assert.Equal(t, 1, other.ArticleIndex(ctx))
}

// TestArticleIndexWithContextCache covers the request level cache the article indexes
// of an owner are resolved through: a page rendering many links reads them once, and a
// write that renumbers them drops the cached ones.
func TestArticleIndexWithContextCache(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := cache.WithCacheContext(t.Context())

	subject, err := repo_model.GetOrCreateSubject(ctx, "Cached Index Subject")
	assert.NoError(t, err)
	otherSubject, err := repo_model.GetOrCreateSubject(ctx, "Cached Index Other Subject")
	assert.NoError(t, err)

	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	other, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, repo.OwnerID, other.OwnerID)

	repo.SubjectID = subject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id"))
	other.SubjectID = subject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, other, "subject_id"))

	// the indexes of one owner are numbered per subject, so the articles of a second
	// subject start at 1 again
	third, err := repo_model.GetRepositoryByID(ctx, 3)
	assert.NoError(t, err)
	third.SubjectID = otherSubject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, third, "subject_id"))
	assert.Equal(t, 1, third.ArticleIndex(ctx))

	first, second := other.ArticleIndex(ctx), repo.ArticleIndex(ctx)
	assert.Equal(t, 1, first)
	assert.Equal(t, 2, second)
	// repeating the lookups is served from the cache and agrees with itself
	assert.Equal(t, first, other.ArticleIndex(ctx))
	assert.Equal(t, second, repo.ArticleIndex(ctx))

	// archiving the article at the first position renumbers both, and the cached
	// indexes must not survive that write
	assert.NoError(t, repo_model.SetArchiveRepoState(ctx, other, true))
	assert.Equal(t, 1, repo.ArticleIndex(ctx))
	assert.Equal(t, 2, other.ArticleIndex(ctx))
}

// TestRepositoryLinkTombstoneUsesSubject documents that a deleted article keeps an
// address inside the subject hierarchy, sorted behind every article the owner can
// still show, so the deletion notice is served where the article used to be.
func TestRepositoryLinkTombstoneUsesSubject(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Tombstone Routing Subject")
	assert.NoError(t, err)

	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	repo.SubjectID = subject.ID
	repo.IsTombstoned = true
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id", "is_tombstoned"))

	base := setting.AppSubURL + "/subject/Tombstone%20Routing%20Subject/" + repo.OwnerName

	// the owner's only article for the subject carries no index, deleted or not
	assert.Equal(t, base, repo.Link())

	// a tombstone never stands in for an article the owner can still show, so an
	// active one takes the first position and the tombstone spells out its index
	other, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, repo.OwnerID, other.OwnerID)
	other.SubjectID = subject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, other, "subject_id"))

	assert.Equal(t, base+"/2", repo.Link())
	assert.Equal(t, 2, repo.ArticleIndex(ctx))
	assert.Equal(t, 1, other.ArticleIndex(ctx))

	found, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, repo.OwnerName, "Tombstone Routing Subject")
	assert.NoError(t, err)
	assert.Equal(t, other.ID, found.ID)

	found, err = repo_model.GetRepositoryByOwnerSubjectAndIndex(ctx, repo.OwnerName, "Tombstone Routing Subject", 2)
	assert.NoError(t, err)
	assert.Equal(t, repo.ID, found.ID)
}

// TestSubjectLookupPrefersActiveRepository documents that the vanity url of a subject
// keeps pointing at the owner's active article, even when an archived repository of the
// same owner is named exactly like the subject.
func TestSubjectLookupPrefersActiveRepository(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "physics")
	assert.NoError(t, err)

	// the archived repository is named exactly like the subject, which is what
	// GenerateRepoNameFromSubject produces for a single word subject
	archived, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	archived.SubjectID = subject.ID
	archived.Name = "physics"
	archived.LowerName = "physics"
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, archived, "subject_id", "name", "lower_name"))
	assert.NoError(t, repo_model.SetArchiveRepoState(ctx, archived, true))

	active, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, archived.OwnerID, active.OwnerID)
	active.SubjectID = subject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, active, "subject_id"))

	found, err := repo_model.GetRepositoryByOwnerAndSubject(ctx, archived.OwnerName, "physics")
	assert.NoError(t, err)
	assert.Equal(t, active.ID, found.ID)

	// the archived article sits behind the active one in the subject hierarchy, so its
	// link carries the index that tells the two apart
	assert.Equal(t, found.Link()+"/2", archived.Link())
	assert.NotEqual(t, found.Link(), archived.Link())
}

// TestRepositoryHTMLURLResolvesArticleIndexOnContext covers the article index lookup
// HTMLURL performs: it must run on the context it is given. A caller inside a write
// transaction, such as the webhook payload built while an issue is created, would
// otherwise have the index read on a second connection, which blocks until the
// transaction it is nested in commits and deadlocks the request.
func TestRepositoryHTMLURLResolvesArticleIndexOnContext(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	subject, err := repo_model.GetOrCreateSubject(ctx, "Transactional Index Subject")
	assert.NoError(t, err)

	repo, err := repo_model.GetRepositoryByID(ctx, 1)
	assert.NoError(t, err)
	repo.SubjectID = subject.ID
	assert.NoError(t, repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "subject_id"))

	other, err := repo_model.GetRepositoryByID(ctx, 2)
	assert.NoError(t, err)
	assert.Equal(t, repo.OwnerID, other.OwnerID)

	// the owner's only article for the subject carries no index
	assert.NotEmpty(t, repo.HTMLURL(ctx))
	assert.NotContains(t, repo.HTMLURL(ctx), "/2")

	// inside the transaction the second article is only visible to the transaction
	// itself, so the index resolved there can only be the uncommitted one
	assert.NoError(t, db.WithTx(ctx, func(ctx context.Context) error {
		other.SubjectID = subject.ID
		if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, other, "subject_id"); err != nil {
			return err
		}
		if err := repo_model.SetArchiveRepoState(ctx, repo, true); err != nil {
			return err
		}
		assert.Equal(t, 2, repo.ArticleIndex(ctx))
		assert.True(t, strings.HasSuffix(repo.HTMLURL(ctx), "/2"), repo.HTMLURL(ctx))
		return nil
	}))
}
