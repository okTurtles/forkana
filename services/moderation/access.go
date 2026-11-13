// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"
	"fmt"

	moderation_model "code.gitea.io/gitea/models/moderation"
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
