// Copyright 2026 The Forkana Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// The cases of web_src/js/components/graph/article-summary.test.ts: the Read tab's
// summary must be the one the Bubble and Table views show.
func TestArticleSummary(t *testing.T) {
	cases := []struct{ name, md, want string }{
		{"front matter, heading, fence, links and emphasis", "---\ntitle: Physics\n---\n\n# Physics\n\n```math\nE=mc^2\n```\n\n**Physics** is the study of [matter](https://x.example).", "Physics is the study of matter."},
		{"typical wiki2md article", "---\ntitle: Mars\nsource: wikipedia\n---\n# Mars\n\nMars is the fourth planet from the Sun.\n\nSecond paragraph.", "Mars is the fourth planet from the Sun."},
		{"hard-wrapped paragraph", "First line\nsecond line.\n\nNot included.", "First line second line."},
		{"images, tables and HTML first", "![logo](x.png)\n\n| a | b |\n|---|---|\n\n<div>x</div>\n\nThe text.", "The text."},
		{"CRLF and BOM", "\ufeff---\r\ntitle: X\r\n---\r\n\r\nHello world.\r\n", "Hello world."},
		{"leading thematic break", "---\n\nReal text.", "Real text."},
		{"leading starred thematic break", "* * *\n\nReal text.", "Real text."},
		{"unclosed front matter", "---\ntitle: X\n", "title: X"},
		{"empty", "", ""},
		{"whitespace only", "\n\n  \n", ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, articleSummary(c.md), c.name)
	}

	long := articleSummary(strings.TrimSpace(strings.Repeat("word ", 200)))
	assert.LessOrEqual(t, utf8.RuneCountInString(long), articleSummaryMaxLength)
	assert.True(t, strings.HasSuffix(long, "…"))
}
