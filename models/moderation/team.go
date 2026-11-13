// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"context"
	"strings"

	"code.gitea.io/gitea/models/db"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"

	"xorm.io/builder"
)

// IsModeratorsTeamMember reports whether the user is a member of the moderators team
// ([moderation] ORG / TEAM). It is one query, by name, so that it needs no cache to invalidate
// when setup creates the team.
func IsModeratorsTeamMember(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return db.GetEngine(ctx).Table("team_user").
		Join("INNER", "team", "team.id = team_user.team_id").
		Join("INNER", "`user`", "`user`.id = team.org_id").
		Where(builder.Eq{
			"team_user.uid":     userID,
			"team.lower_name":   strings.ToLower(setting.Moderation.Team),
			"`user`.lower_name": strings.ToLower(setting.Moderation.Org),
			"`user`.type":       user_model.UserTypeOrganization,
		}).
		Exist()
}
