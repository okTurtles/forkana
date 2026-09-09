// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"

	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/modules/timeutil"
)

// TombstoneRepository turns a repository into a tombstone instead of deleting it.
//
// The row and the on-disk git data are both kept: the surviving forks need the
// commit history to keep a valid ancestor and to render the point of contention
// where they diverged. Everything that would expose the article itself (file
// browser, raw files, archives, API contents, git transport) is blocked while the
// repository is tombstoned, and the article no longer appears in any listing.
func TombstoneRepository(ctx context.Context, repo *repo_model.Repository) error {
	if repo.IsTombstoned {
		return nil
	}

	repo.IsTombstoned = true
	repo.TombstonedUnix = timeutil.TimeStampNow()
	if err := repo_model.UpdateRepositoryColsNoAutoTime(ctx, repo, "is_tombstoned", "tombstoned_unix"); err != nil {
		return fmt.Errorf("tombstone repository %d: %w", repo.ID, err)
	}
	return nil
}

// repoHasForks reports whether the repository still has descendants that would
// lose their ancestry if the repository were hard-deleted.
func repoHasForks(ctx context.Context, repo *repo_model.Repository) (bool, error) {
	hasForks, err := repo_model.HasForks(ctx, repo.ID)
	if err != nil {
		return false, fmt.Errorf("check forks of repository %d: %w", repo.ID, err)
	}
	return hasForks, nil
}

// CanBeTombstoneDeleted reports whether deleting the repository would tombstone it
// rather than remove it. It is used by the UI to warn the author up front.
func CanBeTombstoneDeleted(ctx context.Context, repo *repo_model.Repository) (bool, error) {
	if repo.IsTombstoned {
		return false, nil
	}
	return repoHasForks(ctx, repo)
}

// deleteOrTombstoneRepository hard-deletes the repository when nothing was forked
// from it, and tombstones it otherwise. It reports whether a tombstone was created.
func deleteOrTombstoneRepository(ctx context.Context, repo *repo_model.Repository) (tombstoned bool, err error) {
	hasForks, err := repoHasForks(ctx, repo)
	if err != nil {
		return false, err
	}
	if !hasForks {
		// DeleteRepositoryDirectly opens its own transaction and removes the git data,
		// so it must not run inside a surrounding transaction here.
		return false, DeleteRepositoryDirectly(ctx, repo.ID)
	}
	return true, TombstoneRepository(ctx, repo)
}
