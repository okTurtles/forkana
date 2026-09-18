// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/optional"
	"code.gitea.io/gitea/modules/setting"
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

// WouldBeTombstonedOnDelete reports whether deleting the repository would leave a
// tombstone behind rather than remove it, because other articles were forked from it.
// It is used by the UI to warn the author up front, which is why an existing tombstone
// reports false: its deletion notice has already been shown.
func WouldBeTombstonedOnDelete(ctx context.Context, repo *repo_model.Repository) (bool, error) {
	if repo.IsTombstoned {
		return false, nil
	}
	return repoHasForks(ctx, repo)
}

// DeleteOutcome reports what a deletion request did to the repository. It is not a
// success flag: callers must tell an effective deletion from a request that changed
// nothing, so that they do not report a removal that never happened.
type DeleteOutcome int

const (
	// RepositoryDeleted means the repository row and its git data were removed.
	RepositoryDeleted DeleteOutcome = iota
	// RepositoryTombstoned means the repository was turned into a tombstone because
	// forks still depend on its history.
	RepositoryTombstoned
	// RepositoryAlreadyTombstoned means nothing happened: the repository already was a
	// tombstone and its forks still depend on it, so it cannot be removed yet.
	RepositoryAlreadyTombstoned
)

// deleteOrTombstoneRepository hard-deletes the repository when nothing was forked
// from it, and tombstones it otherwise.
func deleteOrTombstoneRepository(ctx context.Context, repo *repo_model.Repository) (DeleteOutcome, error) {
	hasForks, err := repoHasForks(ctx, repo)
	if err != nil {
		return RepositoryDeleted, err
	}
	if !hasForks {
		// Once the last fork is gone nothing depends on the history any more, so an
		// existing tombstone is removed here too.
		// DeleteRepositoryDirectly opens its own transaction and removes the git data,
		// so it must not run inside a surrounding transaction here.
		return RepositoryDeleted, DeleteRepositoryDirectly(ctx, repo.ID)
	}
	if repo.IsTombstoned {
		return RepositoryAlreadyTombstoned, nil
	}
	return RepositoryTombstoned, TombstoneRepository(ctx, repo)
}

// OwnsTombstones reports whether the owner still owns tombstoned repositories,
// which have to outlive the account itself.
func OwnsTombstones(ctx context.Context, ownerID int64) (bool, error) {
	count, err := repo_model.CountRepositories(ctx, repo_model.CountRepositoryOptions{
		OwnerID:    ownerID,
		Tombstoned: optional.Some(true),
	})
	if err != nil {
		return false, fmt.Errorf("count tombstones of owner %d: %w", ownerID, err)
	}
	return count > 0, nil
}

// AnonymizeTombstoneOwner strips the identity from an owner whose account is being
// deleted while it still owns tombstones. The row cannot go away with the account:
// Repository.OwnerID is not nullable and the git data lives under the owner's
// directory, so removing it would orphan every tombstone and break the ancestry of
// the forks that depend on them. The owner name is rewritten on the repositories as
// well, so the caller only has to move the on-disk directory to owner.Name.
func AnonymizeTombstoneOwner(ctx context.Context, owner *user_model.User) error {
	newName := fmt.Sprintf("deleted-user-%d", owner.ID)
	fullName := "Deleted user"
	if owner.IsOrganization() {
		newName = fmt.Sprintf("deleted-org-%d", owner.ID)
		fullName = "Deleted organization"
	}

	// A redirect from any previous name would defeat the anonymization.
	if _, err := db.DeleteByBean(ctx, &user_model.Redirect{RedirectUserID: owner.ID}); err != nil {
		return fmt.Errorf("delete redirects of owner %d: %w", owner.ID, err)
	}

	owner.Name = newName
	owner.LowerName = strings.ToLower(newName)
	owner.FullName = fullName
	owner.Email = newName + "@" + setting.Service.NoReplyAddress
	owner.KeepEmailPrivate = true
	owner.Passwd = ""
	owner.Salt = ""
	owner.Rands = ""
	owner.LoginName = ""
	owner.Avatar = ""
	owner.AvatarEmail = ""
	owner.UseCustomAvatar = false
	owner.Location = ""
	owner.Website = ""
	owner.Description = ""
	owner.IsActive = false
	owner.IsAdmin = false
	owner.ProhibitLogin = true

	if err := user_model.UpdateUserCols(ctx, owner,
		"name", "lower_name", "full_name", "email", "keep_email_private",
		"passwd", "salt", "rands", "login_name", "avatar", "avatar_email",
		"use_custom_avatar", "location", "website", "description",
		"is_active", "is_admin", "prohibit_login",
	); err != nil {
		return fmt.Errorf("anonymize owner %d: %w", owner.ID, err)
	}

	if err := repo_model.UpdateRepositoryOwnerNames(ctx, owner.ID, newName); err != nil {
		return fmt.Errorf("update repository owner names of owner %d: %w", owner.ID, err)
	}
	return nil
}
