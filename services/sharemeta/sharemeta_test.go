// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package sharemeta

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestFirstParagraph(t *testing.T) {
	cases := []struct {
		name, markdown, expected string
	}{
		{"empty", "", ""},
		{"plain", "The Moon is Earth's only natural satellite.", "The Moon is Earth's only natural satellite."},
		{
			"skips the headings, the images and the html",
			"# Moon\n\n![The Moon](moon.png)\n\n<div>banner</div>\n\n## Overview\n\nThe **Moon** is\nEarth's *only* natural satellite.\n\nSecond paragraph.",
			"The Moon is Earth's only natural satellite.",
		},
		{
			"keeps the link text, drops the footnotes and the inline images",
			"See [the Sun](https://example.com/sun) ![icon](i.png) and `code`.[^1]\n\n[^1]: A note.",
			"See the Sun and code.",
		},
		{"skips the lists and the quotes", "- one\n- two\n\n> quoted\n\nText.", "Text."},
		{"a document without a paragraph", "# Title\n\n- item", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, FirstParagraph([]byte(c.markdown)))
		})
	}
}

func TestEllipsis(t *testing.T) {
	assert.Equal(t, "short", Ellipsis("short", 200))

	long := strings.Repeat("word ", 60)
	cut := Ellipsis(long, 200)
	assert.LessOrEqual(t, utf8.RuneCountInString(cut), 200)
	assert.True(t, strings.HasSuffix(cut, "word…"), cut)

	unbroken := strings.Repeat("é", 300)
	cut = Ellipsis(unbroken, 200)
	assert.Equal(t, 200, utf8.RuneCountInString(cut))
}
