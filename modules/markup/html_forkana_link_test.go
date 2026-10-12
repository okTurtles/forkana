// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"testing"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
)

func TestParseForkanaInternalLink(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/")()
	defer test.MockVariableValue(&setting.AppSubURL, "")()

	cases := []struct {
		href string
		ok   bool
		want ForkanaInternalLink
	}{
		{"subject/Solar System", true, ForkanaInternalLink{Path: "subject/Solar%20System", Subject: "Solar System"}},
		{"subject/Solar%20System/pierre", true, ForkanaInternalLink{Path: "subject/Solar%20System/pierre", Subject: "Solar System", Owner: "pierre"}},
		{"subject/Solar System/pierre/2", true, ForkanaInternalLink{Path: "subject/Solar%20System/pierre/2", Subject: "Solar System", Owner: "pierre"}},
		{"subject/AC%2FDC", true, ForkanaInternalLink{Path: "subject/AC%2FDC", Subject: "AC/DC"}},
		{"subject/What?", true, ForkanaInternalLink{Path: "subject/What%3F", Subject: "What?"}},
		{"subject/Moon#history", true, ForkanaInternalLink{Path: "subject/Moon", Suffix: "#history", Subject: "Moon"}},
		{"/subject/Moon", true, ForkanaInternalLink{Path: "subject/Moon", Subject: "Moon"}},
		{"https://forkana.example/subject/Solar%20System", true, ForkanaInternalLink{Path: "subject/Solar%20System", Subject: "Solar System"}},
		{"community-notes", true, ForkanaInternalLink{Path: "community-notes"}},
		{"explore/subjects?q=moon", true, ForkanaInternalLink{Path: "explore/subjects", Suffix: "?q=moon"}},
		{"subject", false, ForkanaInternalLink{}},
		{"subject/", false, ForkanaInternalLink{}},
		{"subject//pierre", false, ForkanaInternalLink{}},
		{"notes.md", false, ForkanaInternalLink{}},
		{"https://example.com/subject/Moon", false, ForkanaInternalLink{}},
		{"#subject", false, ForkanaInternalLink{}},
		{"/user2", false, ForkanaInternalLink{}},
	}
	for _, c := range cases {
		got, ok := ParseForkanaInternalLink(c.href)
		assert.Equal(t, c.ok, ok, c.href)
		if c.ok {
			assert.Equal(t, c.want, got, c.href)
		}
	}
}

func TestIsExternalLink(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/")()
	assert.True(t, isExternalLink("https://www.nasa.gov/"))
	assert.True(t, isExternalLink("http://example.com/a?b#c"))
	assert.False(t, isExternalLink("https://forkana.example/explore"))
	assert.False(t, isExternalLink("mailto:a@b.c"))
	assert.False(t, isExternalLink("subject/Moon"))
}
