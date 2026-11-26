// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"strings"

	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"

	"xorm.io/builder"
)

// IsModerationCasesRepo reports whether repo is the private repository moderation keeps its
// reports and cases in ([moderation] ORG / REPO). It goes by name, so it holds whether or not
// moderation is enabled, which lets the repository be prepared before it is.
//
// It lives here rather than in models/moderation so that the repository and organization
// services, which models/moderation's callers import, can use it.
func (repo *Repository) IsModerationCasesRepo() bool {
	return repo != nil &&
		strings.EqualFold(repo.OwnerName, setting.Moderation.Org) &&
		strings.EqualFold(repo.Name, setting.Moderation.Repo)
}

// AutoWatchRepo makes u watch repo, as Gitea does when a repository is created, transferred
// or given to a team. It does nothing for the moderation cases repository, which stays
// unwatched: moderators get targeted alerts, not a notification for every report. Every
// automatic watch goes through it; a user watching the repository on purpose doesn't.
func AutoWatchRepo(ctx context.Context, u *user_model.User, repo *Repository) error {
	if repo.IsModerationCasesRepo() {
		return nil
	}
	return WatchRepo(ctx, u, repo, true)
}

// ModerationOrgIDCond selects the ID of the moderation organization ([moderation] ORG). Like
// IsModerationCasesRepo it goes by name only: user and organization names share one namespace.
func ModerationOrgIDCond() *builder.Builder {
	return builder.Select("`user`.id").From("`user`").Where(builder.Eq{
		"`user`.lower_name": strings.ToLower(setting.Moderation.Org),
	})
}

// moderationCasesRepoCond matches the moderation cases repository ([moderation] ORG / REPO).
func moderationCasesRepoCond() builder.Cond {
	return builder.Eq{"repository.lower_name": strings.ToLower(setting.Moderation.Repo)}.
		And(builder.In("repository.owner_id", ModerationOrgIDCond()))
}

// ModerationCasesRepoIDCond selects the ID of the moderation cases repository.
func ModerationCasesRepoIDCond() *builder.Builder {
	return builder.Select("repository.id").From("repository").Where(moderationCasesRepoCond())
}

// notModerationCasesRepoCond excludes the moderation cases repository from a listing: its
// issues are reached through the moderation pages, never by browsing repositories.
func notModerationCasesRepoCond() builder.Cond {
	return builder.Not{moderationCasesRepoCond()}
}
