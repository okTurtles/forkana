// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package help

import (
	"fmt"
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReasonExampleCounts: the example counts given to reason() match the en-US strings. Too high
// shows a raw key as an example, too low drops one.
func TestReasonExampleCounts(t *testing.T) {
	locale, err := setting.NewConfigProviderFromFile("../../../custom/options/locale/locale_en-US.ini")
	require.NoError(t, err)
	help := locale.Section("help")
	has := func(key string) bool { return help.HasKey(strings.TrimPrefix(key, "help.")) }

	for _, group := range reasonGroups() {
		for _, r := range group.Reasons {
			for _, key := range append(append([]string{}, r.NotAllowed...), r.Allowed...) {
				assert.True(t, has(key), "missing string %s", key)
			}
			assert.False(t, has(fmt.Sprintf("help.report.%s.not_allowed_%d", r.Key, len(r.NotAllowed)+1)), "%s has more Not allowed examples", r.Key)
			assert.False(t, has(fmt.Sprintf("help.report.%s.allowed_%d", r.Key, len(r.Allowed)+1)), "%s has more Allowed examples", r.Key)
		}
	}
}
