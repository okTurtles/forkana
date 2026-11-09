// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package admin

import (
	"strconv"
	"testing"

	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/services/contexttest"
	"code.gitea.io/gitea/services/forms"

	"github.com/stretchr/testify/assert"
)

// TestEditUserPost_ForkOnly checks that a site admin marks a user fork-only (#156) from
// the user's edit page, and clears it again.
func TestEditUserPost_ForkOnly(t *testing.T) {
	unittest.PrepareTestEnv(t)
	admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1, IsAdmin: true})
	target := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	assert.False(t, target.IsForkOnly)

	edit := func(forkOnly bool) {
		ctx, _ := contexttest.MockContext(t, "admin/users/4/edit")
		ctx.Doer = admin
		ctx.SetPathParam("userid", strconv.FormatInt(target.ID, 10))
		web.SetForm(ctx, &forms.AdminEditUserForm{
			LoginType:               "0-0",
			UserName:                target.Name,
			LoginName:               target.LoginName,
			FullName:                target.FullName,
			Email:                   target.Email,
			MaxRepoCreation:         target.MaxRepoCreation,
			Active:                  target.IsActive,
			AllowCreateOrganization: target.AllowCreateOrganization,
			Visibility:              target.Visibility,
			ForkOnly:                forkOnly,
		})
		EditUserPost(ctx)
		assert.NotEmpty(t, ctx.Flash.SuccessMsg)
	}

	edit(true)
	u := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: target.ID})
	assert.True(t, u.IsForkOnly)
	assert.True(t, u.IsActive, "the other flags are kept")

	edit(false)
	u = unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: target.ID})
	assert.False(t, u.IsForkOnly)
}
