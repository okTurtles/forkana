// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package subjecttitle

import (
	"os"
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sharedCases are the cases of testdata/cases.json, which web_src/js/features/subject-title.test.ts
// also runs against the JavaScript mirror of the rule.
type sharedCases struct {
	Normalize []struct {
		In  string `json:"in"`
		Out string `json:"out"`
	} `json:"normalize"`
	Check []struct {
		Title   string  `json:"title"`
		Repeat  int     `json:"repeat"`
		Problem Problem `json:"problem"`
	} `json:"check"`
}

func loadSharedCases(t *testing.T) sharedCases {
	data, err := os.ReadFile("testdata/cases.json")
	require.NoError(t, err)
	var cases sharedCases
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases.Normalize)
	require.NotEmpty(t, cases.Check)
	return cases
}

func TestNormalizeSharedCases(t *testing.T) {
	for _, c := range loadSharedCases(t).Normalize {
		assert.Equal(t, c.Out, Normalize(c.In), "Normalize(%q)", c.In)
	}
}

func TestCheckSharedCases(t *testing.T) {
	for _, c := range loadSharedCases(t).Check {
		title := c.Title
		if c.Repeat > 0 {
			title = strings.Repeat(title, c.Repeat)
		}
		require.Equal(t, title, Normalize(title), "check cases must be normalized: %q", title)
		assert.Equal(t, c.Problem, Check(title), "Check(%q)", title)
		assert.Equal(t, c.Problem == ProblemNone, IsValid(title), "IsValid(%q)", title)
	}
}

func TestCheckInvalidUTF8(t *testing.T) {
	// Wikipedia: "Titles cannot contain invalid UTF-8 sequences", such as the encoding of the
	// unpaired surrogate U+D800 or of U+180000 (beyond U+10FFFF). JSON cannot hold them, so
	// they are tested here only; the JavaScript test checks lone surrogates.
	for _, title := range []string{"Foo\xed\xa0\x80", "Foo\xf6\x80\x80\x80", "Foo\xff"} {
		assert.Equal(t, ProblemForbiddenChar, Check(title), "Check(%q)", title)
	}
}

func TestIsTooLong(t *testing.T) {
	// fewer than 256 bytes in UTF-8, as on Wikipedia
	assert.False(t, IsTooLong(strings.Repeat("a", 255)))
	assert.True(t, IsTooLong(strings.Repeat("a", 256)))
	assert.False(t, IsTooLong(strings.Repeat("é", 127)), "254 bytes")
	assert.True(t, IsTooLong(strings.Repeat("é", 128)), "256 bytes")
}

func TestNormalizeIsIdempotent(t *testing.T) {
	for _, c := range loadSharedCases(t).Normalize {
		once := Normalize(c.In)
		assert.Equal(t, once, Normalize(once), "Normalize(Normalize(%q))", c.In)
	}
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		// valid titles only get normalized
		"Moon":                          "Moon",
		";alskdjf":                      ";alskdjf",
		"Test: Gaudí":                   "Test: Gaudí",
		"Python (programming language)": "Python (programming language)",
		"C++":                           "C++",
		"Notepad++":                     "Notepad++",
		"AC/DC":                         "AC/DC",
		"St. Louis":                     "St. Louis",
		"'Til Tuesday":                  "'Til Tuesday",
		"iPhone":                        "IPhone",
		"  Hello_World  ":               "Hello World",
		"100% Pure":                     "100% Pure",
		"AT&T":                          "AT&T",
		// forbidden characters are removed, control characters become spaces
		"C#":                 "C",
		"Song #3":            "Song 3",
		"M|A|R|R|S":          "MARRS",
		"[title of show]":    "Title of show",
		"Red {an orchestra}": "Red an orchestra",
		"While(1<2)":         "While(12)",
		"Foo\tBar\nBaz":      "Foo Bar Baz",
		"Foo\ufffdBar":       "FooBar",
		"Foo\xffBar":         "FooBar",
		// forbidden sequences are broken up
		"%41":               "% 41",
		"50%Fee":            "50% Fee",
		"50%Off":            "50%Off", // "Of" is not hexadecimal
		"AT&amp;T":          "AT&ampT",
		"&#47;pol/":         "&47pol/", // "#" is removed first
		"&#x2F;pol/":        "&x2Fpol/",
		"&a;b;":             "&ab",
		"~~~":               "~~",
		"Tilde ~~~~~ Tilde": "Tilde ~~ Tilde",
		// relative paths and leading colons are removed
		"./Foo":            "Foo",
		"../../Foo":        "Foo",
		"Foo/./Bar/../Baz": "Foo/Bar/Baz",
		"Foo/..":           "Foo",
		"/.":               "",
		".":                "",
		"..":               "",
		":Foo":             "Foo",
		":: : Foo":         "Foo",
		// nothing valid left
		"":         "",
		"   ":      "",
		"#<>[]|{}": "",
	}
	for in, want := range cases {
		got := Clean(in)
		assert.Equal(t, want, got, "Clean(%q)", in)
		if got != "" {
			assert.True(t, IsValid(got), "Clean(%q) = %q must be valid", in, got)
		}
	}

	// cut at MaxBytes without splitting a character
	long := Clean(strings.Repeat("É", 200))
	assert.Equal(t, strings.Repeat("É", 127), long)
	assert.True(t, IsValid(long))
	long = Clean(strings.Repeat("ab ", 200))
	assert.LessOrEqual(t, len(long), MaxBytes)
	assert.True(t, IsValid(long))
}
