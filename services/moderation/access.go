// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"
	"fmt"

	moderation_model "code.gitea.io/gitea/models/moderation"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
)

// IsModerator reports whether the user can use the moderation workspace: moderation is
// enabled, and the user is a site administrator or a member of the moderators team.
func IsModerator(ctx context.Context, user *user_model.User) (bool, error) {
	if !setting.Moderation.Enabled || user == nil || user.ID <= 0 || !user.IsActive || user.ProhibitLogin {
		return false, nil
	}
	if user.IsAdmin {
		return true, nil
	}
	member, err := moderation_model.IsModeratorsTeamMember(ctx, user.ID)
	if err != nil {
		return false, fmt.Errorf("check moderators team membership of user %d: %w", user.ID, err)
	}
	return member, nil
}

// GetCasesRepo returns the cases repository ([moderation] ORG / REPO). It returns a
// repo_model.ErrRepoNotExist error when setup hasn't created it yet.
func GetCasesRepo(ctx context.Context) (*repo_model.Repository, error) {
	return repo_model.GetRepositoryByOwnerAndName(ctx, setting.Moderation.Org, setting.Moderation.Repo)
}
