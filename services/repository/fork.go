// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"code.gitea.io/gitea/models/db"
	git_model "code.gitea.io/gitea/models/git"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/gitrepo"
	"code.gitea.io/gitea/modules/log"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	notify_service "code.gitea.io/gitea/services/notify"

	"golang.org/x/sync/errgroup"
	"xorm.io/builder"
)

// ForkOnEditPermissions contains the permission state for fork-on-edit workflow.
// This struct consolidates all permission checks needed to determine how a user
// can edit a repository they don't own.
type ForkOnEditPermissions struct {
	// IsRepoOwner is true if the user owns the repository
	IsRepoOwner bool
	// CanEditDirectly is true if the user can commit directly to the repository
	CanEditDirectly bool
	// NeedsFork is true if the user needs to create a fork to edit
	NeedsFork bool
	// HasExistingFork is true if the user already has a fork of this repository
	HasExistingFork bool
	// ExistingFork is the user's existing fork (nil if none)
	ExistingFork *repo_model.Repository
	// BlockedBySubject is true if the user already owns a different repo for the same subject
	BlockedBySubject bool
	// OwnRepoForSubject is the user's existing repo for the subject (nil if none)
	OwnRepoForSubject *repo_model.Repository
}

// CheckForkOnEditPermissions determines the user's editing permissions for a repository.
// It checks ownership, subject ownership restrictions, and existing forks.
// Returns an empty permissions struct if doer is nil (not signed in).
func CheckForkOnEditPermissions(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) (*ForkOnEditPermissions, error) {
	perms := &ForkOnEditPermissions{}

	// Not signed in - no permissions
	if doer == nil {
		return perms, nil
	}

	// Check if user owns the repository
	if repo.OwnerID == doer.ID {
		perms.IsRepoOwner = true
		perms.CanEditDirectly = true
		return perms, nil
	}

	// Run subject ownership check and fork detection in parallel.
	// These queries are independent and can be executed concurrently.
	var ownRepo *repo_model.Repository
	var existingFork *repo_model.Repository

	g, gCtx := errgroup.WithContext(ctx)

	// Check if user owns a different repository for the same subject
	if repo.SubjectID > 0 {
		g.Go(func() error {
			var err error
			ownRepo, err = repo_model.GetRepositoryByOwnerIDAndSubjectID(gCtx, doer.ID, repo.SubjectID)
			return err
		})
	}

	// Check for existing fork
	g.Go(func() error {
		var err error
		existingFork, err = repo_model.GetForkedRepo(gCtx, doer.ID, repo.ID)
		return err
	})

	// Wait for both queries to complete
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Process subject ownership result
	if ownRepo != nil && ownRepo.ID != repo.ID {
		// User already owns a different repository for this subject
		perms.BlockedBySubject = true
		perms.OwnRepoForSubject = ownRepo
		return perms, nil
	}

	// Process fork detection result
	if existingFork != nil {
		perms.HasExistingFork = true
		perms.ExistingFork = existingFork
	} else {
		perms.NeedsFork = true
	}

	return perms, nil
}

// ErrForkAlreadyExist represents a "ForkAlreadyExist" kind of error.
type ErrForkAlreadyExist struct {
	Uname    string
	RepoName string
	ForkName string
}

// IsErrForkAlreadyExist checks if an error is an ErrForkAlreadyExist.
func IsErrForkAlreadyExist(err error) bool {
	_, ok := err.(ErrForkAlreadyExist)
	return ok
}

func (err ErrForkAlreadyExist) Error() string {
	return fmt.Sprintf("repository is already forked by user [uname: %s, repo path: %s, fork path: %s]", err.Uname, err.RepoName, err.ForkName)
}

func (err ErrForkAlreadyExist) Unwrap() error {
	return util.ErrAlreadyExist
}

// ErrUserOwnsSubjectRepo represents an error when a user already owns a different
// repository for the same subject and cannot fork/edit another repository for that subject.
type ErrUserOwnsSubjectRepo struct {
	UserID         int64
	SubjectID      int64
	ExistingRepoID int64
}

// IsErrUserOwnsSubjectRepo checks if an error is an ErrUserOwnsSubjectRepo.
func IsErrUserOwnsSubjectRepo(err error) bool {
	var e ErrUserOwnsSubjectRepo
	return errors.As(err, &e)
}

func (err ErrUserOwnsSubjectRepo) Error() string {
	return fmt.Sprintf("user already owns repository for subject [user_id: %d, subject_id: %d, existing_repo_id: %d]",
		err.UserID, err.SubjectID, err.ExistingRepoID)
}

func (err ErrUserOwnsSubjectRepo) Unwrap() error {
	return util.ErrAlreadyExist
}

// ForkRepoOptions contains the fork repository options
type ForkRepoOptions struct {
	BaseRepo     *repo_model.Repository
	Name         string
	Description  string
	SingleBranch string
}

// checkForkTreeSizeLimit checks if the fork tree has reached the maximum size limit.
// Returns nil if the fork is allowed, or ErrForkTreeTooLarge if the limit is exceeded.
//
// The limit is controlled by setting.Repository.MaxForkTreeNodes:
// - If < 0: limit is disabled, always allow forking
// - If = 0: prevent all forking
// - If > 0: limit to this many total nodes in the fork tree
//
// If an error occurs while counting nodes (e.g., database error), the error is logged
// but the fork is allowed to proceed. This ensures that temporary database issues
// don't permanently block fork creation.
func checkForkTreeSizeLimit(ctx context.Context, baseRepo *repo_model.Repository) error {
	limit := setting.Repository.MaxForkTreeNodes

	// Limit disabled
	if limit < 0 {
		return nil
	}

	// Prevent all forking
	if limit == 0 {
		return repo_model.ErrForkTreeTooLarge{Limit: 0}
	}

	// Count nodes in the fork tree
	count, err := repo_model.CountForkTreeNodes(ctx, baseRepo.ID)
	if err != nil {
		// Log the error but don't block fork creation on count errors
		log.Error("Failed to count fork tree nodes for repo %d: %v", baseRepo.ID, err)
		return nil
	}

	// Check if adding one more node would exceed the limit
	if count >= limit {
		return repo_model.ErrForkTreeTooLarge{Limit: limit}
	}

	return nil
}

// ForkRepository forks a repository
func ForkRepository(ctx context.Context, doer, owner *user_model.User, opts ForkRepoOptions) (*repo_model.Repository, error) {
	if err := opts.BaseRepo.LoadOwner(ctx); err != nil {
		return nil, err
	}

	if user_model.IsUserBlockedBy(ctx, doer, opts.BaseRepo.Owner.ID) {
		return nil, user_model.ErrBlockedUser
	}

	// Fork is prohibited, if user has reached maximum limit of repositories
	if !doer.CanForkRepoIn(owner) {
		return nil, repo_model.ErrReachLimitOfRepo{
			Limit: owner.MaxRepoCreation,
		}
	}

	// Check if fork tree has reached maximum size limit
	if err := checkForkTreeSizeLimit(ctx, opts.BaseRepo); err != nil {
		return nil, err
	}

	// Check if user already owns a different repository for the same subject
	// In Forkana, each user should only have one repository per subject
	if opts.BaseRepo.SubjectID > 0 {
		ownRepo, err := repo_model.GetRepositoryByOwnerIDAndSubjectID(ctx, owner.ID, opts.BaseRepo.SubjectID)
		if err != nil {
			return nil, err
		}
		if ownRepo != nil && ownRepo.ID != opts.BaseRepo.ID {
			return nil, ErrUserOwnsSubjectRepo{
				UserID:         owner.ID,
				SubjectID:      opts.BaseRepo.SubjectID,
				ExistingRepoID: ownRepo.ID,
			}
		}
	}

	forkedRepo, err := repo_model.GetUserFork(ctx, opts.BaseRepo.ID, owner.ID)
	if err != nil {
		return nil, err
	}
	if forkedRepo != nil {
		return nil, ErrForkAlreadyExist{
			Uname:    owner.Name,
			RepoName: opts.BaseRepo.FullName(),
			ForkName: forkedRepo.FullName(),
		}
	}

	defaultBranch := opts.BaseRepo.DefaultBranch
	if opts.SingleBranch != "" {
		defaultBranch = opts.SingleBranch
	}

	repo := &repo_model.Repository{
		OwnerID:          owner.ID,
		Owner:            owner,
		OwnerName:        owner.Name,
		Name:             opts.Name,
		LowerName:        strings.ToLower(opts.Name),
		Description:      opts.Description,
		DefaultBranch:    defaultBranch,
		IsPrivate:        opts.BaseRepo.IsPrivate || opts.BaseRepo.Owner.Visibility == structs.VisibleTypePrivate,
		IsEmpty:          opts.BaseRepo.IsEmpty,
		IsFork:           true,
		ForkID:           opts.BaseRepo.ID,
		SubjectID:        opts.BaseRepo.SubjectID,
		ObjectFormatName: opts.BaseRepo.ObjectFormatName,
		Status:           repo_model.RepositoryBeingMigrated,
	}

	// 1 - Create the repository in the database
	err = db.WithTx(ctx, func(ctx context.Context) error {
		if err = createRepositoryInDB(ctx, doer, owner, repo, true); err != nil {
			return err
		}
		if err = repo_model.IncrementRepoForkNum(ctx, opts.BaseRepo.ID); err != nil {
			return err
		}

		// copy lfs files failure should not be ignored
		return git_model.CopyLFS(ctx, repo, opts.BaseRepo)
	})
	if err != nil {
		return nil, err
	}

	// last - clean up if something goes wrong
	// WARNING: Don't override all later err with local variables
	defer func() {
		if err != nil {
			// we can not use the ctx because it maybe canceled or timeout
			cleanupRepository(repo.ID)
		}
	}()

	// 2 - check whether the repository with the same storage exists
	var isExist bool
	isExist, err = gitrepo.IsRepositoryExist(ctx, repo)
	if err != nil {
		log.Error("Unable to check if %s exists. Error: %v", repo.FullName(), err)
		return nil, err
	}
	if isExist {
		log.Error("Files already exist in %s and we are not going to adopt or delete.", repo.FullName())
		// Don't return directly, we need err in defer to cleanupRepository
		err = repo_model.ErrRepoFilesAlreadyExist{
			Uname: repo.OwnerName,
			Name:  repo.Name,
		}
		return nil, err
	}

	// 3 - Clone the repository
	if opts.BaseRepo.IsEmpty {
		if err = gitrepo.InitRepository(ctx, repo, repo.ObjectFormatName); err != nil {
			return nil, fmt.Errorf("InitRepository: %w", err)
		}
	} else {
		cloneCmd := gitcmd.NewCommand("clone", "--bare")
		if opts.SingleBranch != "" {
			cloneCmd.AddArguments("--single-branch", "--branch").AddDynamicArguments(opts.SingleBranch)
		}
		var stdout []byte
		if stdout, _, err = cloneCmd.AddDynamicArguments(opts.BaseRepo.RepoPath(), repo.RepoPath()).
			RunStdBytes(ctx, &gitcmd.RunOpts{Timeout: 10 * time.Minute}); err != nil {
			log.Error("Fork Repository (git clone) Failed for %v (from %v):\nStdout: %s\nError: %v", repo, opts.BaseRepo, stdout, err)
			return nil, fmt.Errorf("git clone: %w", err)
		}
	}

	// 4 - Update the git repository
	if err = updateGitRepoAfterCreate(ctx, repo); err != nil {
		return nil, fmt.Errorf("updateGitRepoAfterCreate: %w", err)
	}

	// 5 - Create hooks
	if err = gitrepo.CreateDelegateHooks(ctx, repo); err != nil {
		return nil, fmt.Errorf("createDelegateHooks: %w", err)
	}

	// 6 - Sync the repository branches and tags
	var gitRepo *git.Repository
	gitRepo, err = gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("OpenRepository: %w", err)
	}
	defer gitRepo.Close()

	if _, err = repo_module.SyncRepoBranchesWithRepo(ctx, repo, gitRepo, doer.ID); err != nil {
		return nil, fmt.Errorf("SyncRepoBranchesWithRepo: %w", err)
	}
	if err = repo_module.SyncReleasesWithTags(ctx, repo, gitRepo); err != nil {
		return nil, fmt.Errorf("Sync releases from git tags failed: %v", err)
	}

	// 7 - Update the repository
	// even if below operations failed, it could be ignored. And they will be retried
	if err = repo_module.UpdateRepoSize(ctx, repo); err != nil {
		log.Error("Failed to update size for repository: %v", err)
		err = nil
	}
	if err = repo_model.CopyLanguageStat(ctx, opts.BaseRepo, repo); err != nil {
		log.Error("Copy language stat from oldRepo failed: %v", err)
		err = nil
	}
	if err = repo_model.CopyLicense(ctx, opts.BaseRepo, repo); err != nil {
		return nil, err
	}

	// 8 - update repository status to be ready
	repo.Status = repo_model.RepositoryReady
	if err = repo_model.UpdateRepositoryColsWithAutoTime(ctx, repo, "status"); err != nil {
		return nil, fmt.Errorf("UpdateRepositoryCols: %w", err)
	}

	notify_service.ForkRepository(ctx, doer, opts.BaseRepo, repo)

	return repo, nil
}

// ConvertForkToNormalRepository convert the provided repo from a forked repo to normal repo
func ConvertForkToNormalRepository(ctx context.Context, repo *repo_model.Repository) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		repo, err := repo_model.GetRepositoryByID(ctx, repo.ID)
		if err != nil {
			return err
		}

		if !repo.IsFork {
			return nil
		}

		if err := repo_model.DecrementRepoForkNum(ctx, repo.ForkID); err != nil {
			log.Error("Unable to decrement repo fork num for old root repo %d of repository %-v whilst converting from fork. Error: %v", repo.ForkID, repo, err)
			return err
		}

		repo.IsFork = false
		repo.ForkID = 0
		return repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "is_fork", "fork_id")
	})
}

// ConvertNormalToForkRepository converts a normal repository to a fork of the specified root repository.
// This is used by the first-article-becomes-root logic when a repository becomes non-empty
// after another repository with the same subject has already become the root.
func ConvertNormalToForkRepository(ctx context.Context, repo *repo_model.Repository, rootRepoID int64) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		// Re-fetch the repo within the transaction to ensure consistency
		repo, err := repo_model.GetRepositoryByID(ctx, repo.ID)
		if err != nil {
			return err
		}

		// Already a fork - nothing to do
		if repo.IsFork {
			return nil
		}

		// Don't try to fork from ourselves
		if repo.ID == rootRepoID {
			return nil
		}

		// Fetch the root repository to check fork tree limits
		rootRepo, err := repo_model.GetRepositoryByID(ctx, rootRepoID)
		if err != nil {
			return err
		}

		// Check if fork tree has reached maximum size limit
		if err := checkForkTreeSizeLimit(ctx, rootRepo); err != nil {
			return err
		}

		// Increment the fork count on the root repository
		if err := repo_model.IncrementRepoForkNum(ctx, rootRepoID); err != nil {
			log.Error("Unable to increment repo fork num for root repo %d when converting repository %-v to fork. Error: %v", rootRepoID, repo, err)
			return err
		}

		// Update this repository to be a fork
		repo.IsFork = true
		repo.ForkID = rootRepoID
		return repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "is_fork", "fork_id")
	})
}

type findForksOptions struct {
	db.ListOptions
	RepoID int64
	Doer   *user_model.User
}

func (opts findForksOptions) ToConds() builder.Cond {
	cond := builder.Eq{"fork_id": opts.RepoID}
	if opts.Doer != nil && opts.Doer.IsAdmin {
		return cond
	}
	return cond.And(repo_model.AccessibleRepositoryCondition(opts.Doer, unit.TypeInvalid))
}

// FindForks returns all the forks of the repository
func FindForks(ctx context.Context, repo *repo_model.Repository, doer *user_model.User, listOptions db.ListOptions) ([]*repo_model.Repository, int64, error) {
	return db.FindAndCount[repo_model.Repository](ctx, findForksOptions{
		ListOptions: listOptions,
		RepoID:      repo.ID,
		Doer:        doer,
	})
}
