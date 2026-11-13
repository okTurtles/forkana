// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"testing"
	"time"

	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadModerationFrom(t *testing.T) {
	t.Run("Defaults", func(t *testing.T) {
		defer test.MockVariableValue(&Moderation)()
		cfg, err := NewConfigProviderFromData(``)
		require.NoError(t, err)

		loadModerationFrom(cfg)
		assert.False(t, Moderation.Enabled)
		assert.Equal(t, "forkana-moderation", Moderation.Org)
		assert.Equal(t, "cases", Moderation.Repo)
		assert.Equal(t, "moderators", Moderation.Team)
		assert.Equal(t, 10, Moderation.MaxAssignedCases)
		assert.Equal(t, 48*time.Hour, Moderation.AssignmentTimeout)
	})

	t.Run("Overrides", func(t *testing.T) {
		defer test.MockVariableValue(&Moderation)()
		cfg, err := NewConfigProviderFromData(`
[moderation]
ENABLED = true
ORG = mods
REPO = reports
TEAM = crew
MAX_ASSIGNED_CASES = 3
ASSIGNMENT_TIMEOUT = 12h
`)
		require.NoError(t, err)

		loadModerationFrom(cfg)
		assert.True(t, Moderation.Enabled)
		assert.Equal(t, "mods", Moderation.Org)
		assert.Equal(t, "reports", Moderation.Repo)
		assert.Equal(t, "crew", Moderation.Team)
		assert.Equal(t, 3, Moderation.MaxAssignedCases)
		assert.Equal(t, 12*time.Hour, Moderation.AssignmentTimeout)
	})

	t.Run("InvalidLimits", func(t *testing.T) {
		defer test.MockVariableValue(&Moderation)()
		cfg, err := NewConfigProviderFromData(`
[moderation]
MAX_ASSIGNED_CASES = 0
ASSIGNMENT_TIMEOUT = -1h
`)
		require.NoError(t, err)

		loadModerationFrom(cfg)
		assert.Equal(t, 1, Moderation.MaxAssignedCases)
		assert.Equal(t, 48*time.Hour, Moderation.AssignmentTimeout)
	})
}
