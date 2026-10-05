// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package subjecttitle

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var validityCases = []struct {
	title string
	valid bool
}{
	// valid
	{"Moon", true},
	{"The Moon", true},
	{"Gaudí", true},
	{"Antoni Gaudí", true},
	{"Zalg'o", true},
	{"O’Brien", true},
	{"Jean-Paul Sartre", true},
	{"1984", true},
	{"2001 A Space Odyssey", true},
	{"Ελλάδα", true},
	{"東京", true},
	{"Москва", true},
	{"हिन्दी", true}, // Devanagari needs combining marks after the first letter
	{"a", true},
	{"Rock 'n' Roll", true},

	// invalid characters
	{"Test: Gaudí", false},
	{"Test: The Gaudí Question", false},
	{"Moon!", false},
	{"C++", false},
	{"AT&T", false},
	{"Foo/Bar", false},
	{"Foo_Bar", false},
	{"Foo.Bar", false},
	{"<script>", false},
	{"Hello 😀", false},
	{"Foo\tBar", false}, // not normalized: tabs are not allowed as such

	// must start with a letter or digit
	{";alskdjf", false},
	{"-Moon", false},
	{"'Moon", false},
	{"’Moon", false},
	{" Moon", false},
	{"́Moon", false}, // a combining mark cannot come first
	{"", false},
}

func TestIsValid(t *testing.T) {
	for _, c := range validityCases {
		assert.Equal(t, c.valid, IsValid(c.title), "IsValid(%q)", c.title)
	}
}

// TestHTMLPattern checks that the HTML pattern used by the forms agrees with IsValid. Go's
// RE2 has the same Unicode classes as the browser's `v` flag.
func TestHTMLPattern(t *testing.T) {
	re := regexp.MustCompile(`^(?:` + HTMLPattern + `)$`)
	for _, c := range validityCases {
		if c.title != Normalize(c.title) {
			// the pattern tolerates whitespace that the server normalizes away
			continue
		}
		assert.Equal(t, c.valid, re.MatchString(c.title), "pattern match %q", c.title)
	}
	assert.True(t, re.MatchString("  The   Moon  "), "surrounding and repeated spaces are normalized by the server")
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Moon":              "Moon",
		"  The Moon  ":      "The Moon",
		"The    Moon":       "The Moon",
		"The \t\n Moon":     "The Moon",
		"   ":               "",
		"Gaudí":            "Gaudí", // NFD → NFC
		" Jean - Paul ":     "Jean - Paul",
		"Antoni  Gaudí   x": "Antoni Gaudí x",
	}
	for in, want := range cases {
		assert.Equal(t, want, Normalize(in), "Normalize(%q)", in)
	}
}

func TestIsTooLong(t *testing.T) {
	assert.False(t, IsTooLong(strings.Repeat("é", MaxLength)), "255 two-byte characters fit")
	assert.True(t, IsTooLong(strings.Repeat("a", MaxLength+1)))
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"Moon":                          "Moon",
		"Antoni Gaudí":                  "Antoni Gaudí",
		"Python (programming language)": "Python programming language",
		"St. Louis":                     "St Louis",
		"Test: The Gaudí Question":      "Test The Gaudí Question",
		";alskdjf":                      "alskdjf",
		"'Til Tuesday":                  "Til Tuesday",
		"Rock 'n' Roll":                 "Rock 'n' Roll",
		"Jean-Paul Sartre":              "Jean-Paul Sartre",
		"C++":                           "C",
		"!!!":                           "",
		"":                              "",
		"  Hello,   World!  ":           "Hello World",
		"- - Dash":                      "Dash",
		"😀😀":                            "",
	}
	for in, want := range cases {
		got := Clean(in)
		assert.Equal(t, want, got, "Clean(%q)", in)
		if got != "" {
			assert.True(t, IsValid(got), "Clean(%q) must be valid", in)
		}
	}

	long := Clean(strings.Repeat("ab ", 200))
	assert.False(t, IsTooLong(long))
	assert.True(t, IsValid(long))
}
