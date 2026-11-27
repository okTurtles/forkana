// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"time"
)

// Moderation settings. Reports, cases and their decisions are issues in one private repository
// (Org/Repo) that only the members of Team and site administrators can see.
var Moderation = struct {
	// Enabled turns moderation on. While it is off, nothing of it is visible: its pages answer
	// 404 and no menu entry, badge or Report action shows.
	Enabled bool
	// Org owns the cases repository and the moderators team.
	Org string
	// Repo is the name of the private repository that holds the reports and cases.
	Repo string
	// Team is the name of the team, in Org, whose members are moderators.
	Team string
	// MaxAssignedCases is how many open cases a moderator can hold at once.
	MaxAssignedCases int
	// AssignmentTimeout is how long a case can stay assigned without activity before it
	// returns to the queue.
	AssignmentTimeout time.Duration
}{
	Enabled:           false,
	Org:               "forkana-moderation",
	Repo:              "cases",
	Team:              "moderators",
	MaxAssignedCases:  10,
	AssignmentTimeout: 48 * time.Hour,
}

func loadModerationFrom(rootCfg ConfigProvider) {
	sec := rootCfg.Section("moderation")
	Moderation.Enabled = sec.Key("ENABLED").MustBool(false)
	Moderation.Org = sec.Key("ORG").MustString("forkana-moderation")
	Moderation.Repo = sec.Key("REPO").MustString("cases")
	Moderation.Team = sec.Key("TEAM").MustString("moderators")
	Moderation.MaxAssignedCases = max(sec.Key("MAX_ASSIGNED_CASES").MustInt(10), 1)
	Moderation.AssignmentTimeout = sec.Key("ASSIGNMENT_TIMEOUT").MustDuration(48 * time.Hour)
	if Moderation.AssignmentTimeout <= 0 {
		Moderation.AssignmentTimeout = 48 * time.Hour
	}
}
