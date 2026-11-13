// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"testing"

	issues_model "code.gitea.io/gitea/models/issues"
	moderation_model "code.gitea.io/gitea/models/moderation"
	"code.gitea.io/gitea/modules/timeutil"

	"github.com/stretchr/testify/assert"
)

func TestDeadlinePassed(t *testing.T) {
	const now = timeutil.TimeStamp(1000)
	sources := &moderation_model.Case{Decision: moderation_model.DecisionSourcesRequested}
	for name, tc := range map[string]struct {
		issue *issues_model.Issue
		c     *moderation_model.Case
		want  bool
	}{
		"Passed":        {&issues_model.Issue{DeadlineUnix: 900}, sources, true},
		"DueNow":        {&issues_model.Issue{DeadlineUnix: now}, sources, true},
		"NotYet":        {&issues_model.Issue{DeadlineUnix: 1100}, sources, false},
		"NoDeadline":    {&issues_model.Issue{}, sources, false},
		"Closed":        {&issues_model.Issue{DeadlineUnix: 900, IsClosed: true}, sources, false},
		"OtherDecision": {&issues_model.Issue{DeadlineUnix: 900}, &moderation_model.Case{Decision: moderation_model.DecisionEscalated}, false},
		"NotDecidedYet": {&issues_model.Issue{DeadlineUnix: 900}, &moderation_model.Case{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, deadlinePassed(tc.issue, tc.c, now))
		})
	}
}
