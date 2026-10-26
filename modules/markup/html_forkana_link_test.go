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
		// the app's own links: a query and a compare page (review #2)
		{"/subject/Moon?view=bubble", true, ForkanaInternalLink{Path: "subject/Moon", Suffix: "?view=bubble", Subject: "Moon"}},
		{"https://forkana.example/subject/Moon/pierre?version=abc#x", true, ForkanaInternalLink{Path: "subject/Moon/pierre", Suffix: "?version=abc#x", Subject: "Moon", Owner: "pierre"}},
		{"/subject/Moon/compare/a...b", true, ForkanaInternalLink{Path: "subject/Moon/compare/a...b", Subject: "Moon"}},
		// the instance's URL with another scheme or host case
		{"http://FORKANA.example/subject/Moon", true, ForkanaInternalLink{Path: "subject/Moon", Subject: "Moon"}},
		{"https://forkana.example.evil/subject/Moon", false, ForkanaInternalLink{}},
		// dot segments: the link would go to another page than the one checked
		{"explore/../subject/Nobody", false, ForkanaInternalLink{}},
		{"subject/Moon/pierre/../../Sun", false, ForkanaInternalLink{}},
		{"subject/./Moon", false, ForkanaInternalLink{}},
		{"subject/%2E%2E/x", false, ForkanaInternalLink{}},
		// a quoted subject name, encoded by the editor
		{"subject/Operation%20%22Desert%20Storm%22", true, ForkanaInternalLink{Path: "subject/Operation%20%22Desert%20Storm%22", Subject: `Operation "Desert Storm"`}},
	}
	for _, c := range cases {
		got, ok := ParseForkanaInternalLink(c.href, true)
		assert.Equal(t, c.ok, ok, c.href)
		if c.ok {
			assert.Equal(t, c.want, got, c.href)
		}
	}

	// outside articles, only subject links are internal (review #3)
	for _, href := range []string{"about", "security/README.md", "explore/notes.md", "/community-notes"} {
		_, ok := ParseForkanaInternalLink(href, false)
		assert.False(t, ok, href)
	}
	_, ok := ParseForkanaInternalLink("subject/Moon", false)
	assert.True(t, ok)
}

func TestIsExternalLink(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/")()
	assert.True(t, isExternalLink("https://www.nasa.gov/"))
	assert.True(t, isExternalLink("http://example.com/a?b#c"))
	assert.False(t, isExternalLink("https://forkana.example/explore"))
	assert.False(t, isExternalLink("mailto:a@b.c"))
	assert.False(t, isExternalLink("subject/Moon"))
	// the instance root
	for _, href := range []string{"https://forkana.example", "https://forkana.example/", "https://forkana.example?x=1", "https://forkana.example#top"} {
		assert.False(t, isExternalLink(href), href)
	}
	assert.True(t, isExternalLink("https://forkana.example.evil"))
	// with user info, the host is not right after "://" (review L1)
	assert.True(t, isExternalLink("https://user@forkana.example/subject/Moon"))
	_, ok := ParseForkanaInternalLink("https://user@forkana.example/subject/Moon", true)
	assert.False(t, ok)

	defer test.MockVariableValue(&setting.AppURL, "https://example.org/forkana/")()
	assert.False(t, isExternalLink("https://example.org/forkana"))
	assert.False(t, isExternalLink("https://example.org/forkana?x=1"))
	assert.True(t, isExternalLink("https://example.org/forkanax"))
	assert.True(t, isExternalLink("https://example.org/"))
}
