// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"sync"
	"testing"
	"time"

	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/models/organization"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/modules/timeutil"
	"code.gitea.io/gitea/modules/util"
	org_service "code.gitea.io/gitea/services/org"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueWarning(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	defer test.MockVariableValue(&setting.Moderation.Enabled, true)()
	defer test.MockVariableValue(&setting.Moderation.Org, "sanction-moderation")()
	ctx := t.Context()

	admin := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	moderator := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	reader := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 5})

	_, err := EnsureSetup(ctx, SetupOptions{Owner: admin})
	require.NoError(t, err)
	org, err := organization.GetOrgByName(ctx, "sanction-moderation")
	require.NoError(t, err)
	team, err := organization.GetTeam(ctx, org.ID, setting.Moderation.Team)
	require.NoError(t, err)
	require.NoError(t, org_service.AddTeamMember(ctx, team, moderator))

	// issues 1 and 2 stand in for case issues: nothing here reads the issue
	for _, id := range []int64{1, 2} {
		require.NoError(t, moderation_model.InsertCase(ctx, &moderation_model.Case{IssueID: id, Kind: moderation_model.CaseKindArticle, TargetRepoID: 1, TargetUserID: owner.ID}))
	}

	t.Run("ReadersCantWarn", func(t *testing.T) {
		_, err := IssueWarning(ctx, WarningOptions{Doer: reader, CaseIssueID: 1})
		require.ErrorIs(t, err, util.ErrPermissionDenied)
	})

	t.Run("UnknownCase", func(t *testing.T) {
		_, err := IssueWarning(ctx, WarningOptions{Doer: moderator, CaseIssueID: 999})
		require.ErrorIs(t, err, util.ErrNotExist)
	})

	t.Run("WarnsForTwelveMonths", func(t *testing.T) {
		before := time.Now()
		s, err := IssueWarning(ctx, WarningOptions{Doer: moderator, CaseIssueID: 1})
		require.NoError(t, err)
		assert.Equal(t, owner.ID, s.UserID)
		assert.Equal(t, moderation_model.SanctionWarning, s.Kind)
		assert.Equal(t, moderation_model.SanctionActive, s.Status)
		ends := s.EndsUnix.AsTime()
		assert.WithinDuration(t, before.AddDate(0, 12, 0), ends, 5*time.Second)

		audit, err := moderation_model.FindAuditByCase(ctx, 1)
		require.NoError(t, err)
		require.Len(t, audit, 1)
		assert.Equal(t, moderation_model.AuditWarned, audit[0].Action)
		assert.Equal(t, moderation_model.ActorModerator, audit[0].ActorRole)
		assert.Equal(t, owner.ID, audit[0].TargetUserID)
		assert.Equal(t, "until:"+ends.UTC().Format(time.DateOnly), audit[0].Summary)

		unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: owner.ID, ProhibitLogin: false})
	})

	t.Run("OnePerCase", func(t *testing.T) {
		_, err := IssueWarning(ctx, WarningOptions{Doer: admin, CaseIssueID: 1})
		require.ErrorIs(t, err, util.ErrAlreadyExist)
	})

	t.Run("ConcurrentWarningsFromOneCase", func(t *testing.T) {
		// SQLite serialises write transactions, so this checks the outcome only; the case lock
		// is what keeps it on PostgreSQL, where both transactions could pass the check at once.
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, doer := range []*user_model.User{admin, moderator} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = IssueWarning(ctx, WarningOptions{Doer: doer, CaseIssueID: 2})
			}()
		}
		wg.Wait()
		failed := 0
		for _, err := range errs {
			if err != nil {
				require.ErrorIs(t, err, util.ErrAlreadyExist)
				failed++
			}
		}
		assert.Equal(t, 1, failed, "exactly one warning from the case")
	})

	t.Run("ActiveWarnings", func(t *testing.T) {
		now := timeutil.TimeStampNow()
		n, err := moderation_model.CountActiveWarnings(ctx, owner.ID, now)
		require.NoError(t, err)
		assert.Equal(t, int64(2), n)

		// expired: still "active" in the row, not in force
		later := timeutil.TimeStamp(time.Now().AddDate(0, 12, 1).Unix())
		n, err = moderation_model.CountActiveWarnings(ctx, owner.ID, later)
		require.NoError(t, err)
		assert.Zero(t, n)

		// overturned or lifted: not in force
		require.NoError(t, moderation_model.InsertSanction(ctx, &moderation_model.Sanction{UserID: owner.ID, Kind: moderation_model.SanctionWarning, Status: moderation_model.SanctionOverturned, EndsUnix: later}))
		// another kind isn't a warning
		require.NoError(t, moderation_model.InsertSanction(ctx, &moderation_model.Sanction{UserID: owner.ID, Kind: moderation_model.SanctionBan, Status: moderation_model.SanctionActive}))
		warnings, err := moderation_model.FindActiveWarnings(ctx, owner.ID, now)
		require.NoError(t, err)
		assert.Len(t, warnings, 2)

		n, err = moderation_model.CountActiveWarnings(ctx, reader.ID, now)
		require.NoError(t, err)
		assert.Zero(t, n)
	})
}
