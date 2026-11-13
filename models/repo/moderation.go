// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
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

// notModerationCasesRepoCond excludes the moderation cases repository from a listing: its
// issues are reached through the moderation pages, never by browsing repositories.
func notModerationCasesRepoCond() builder.Cond {
	return builder.Not{builder.And(
		builder.Eq{"repository.lower_name": strings.ToLower(setting.Moderation.Repo)},
		builder.In("repository.owner_id", builder.Select("`user`.id").From("`user`").Where(builder.Eq{
			"`user`.lower_name": strings.ToLower(setting.Moderation.Org),
			"`user`.type":       user_model.UserTypeOrganization,
		})),
	)}
}
