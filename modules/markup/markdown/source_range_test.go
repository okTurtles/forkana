// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markdown_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/markup/markdown"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sourceRangeArticle = "---\ntitle: Café\n---\n# Açaí 🍓\n\nO açaí é popular — 東京では人気です。\n\n- um ç\n- [x] dois ✅\n  - três 子供\n\n> citação\n> segunda\n\n```go\nfmt.Println(\"olá\")\n```\n\n| a | b |\n| --- | --- |\n| ü | 😀 |\n\n<div>cru</div>\n\n---\n\n## Fim\n\nÚltimo [link](https://example.com) e **negrito**.\n"

var reSourceRange = regexp.MustCompile(`<(\w+)[^>]* data-source-range="(\d+)-(\d+)"`)

func renderWithSourceRanges(t *testing.T, input string, sourceRanges bool) string {
	t.Helper()
	ctx := markup.NewTestRenderContext().WithSourceRanges(sourceRanges)
	out, err := markdown.RenderString(ctx, input)
	require.NoError(t, err)
	return string(out)
}

func TestSourceRangesOffByDefault(t *testing.T) {
	assert.NotContains(t, renderWithSourceRanges(t, sourceRangeArticle, false), "data-source-range")
}

// A fenced code block without attributes must render when source ranges are off: the
// highlighter then hands the renderer nil attributes.
func TestSourceRangesOffCodeBlock(t *testing.T) {
	out := renderWithSourceRanges(t, "```go\nfmt.Println(1)\n```\n", false)
	assert.Contains(t, out, `<pre class="code-block"><code class="chroma language-go display">`)
}

func TestSourceRangesRendered(t *testing.T) {
	out := renderWithSourceRanges(t, sourceRangeArticle, true)
	matches := reSourceRange.FindAllStringSubmatch(out, -1)

	var tags []string
	for _, m := range matches {
		tags = append(tags, m[1])
		start, _ := strconv.Atoi(m[2])
		end, _ := strconv.Atoi(m[3])
		require.Less(t, start, end)
		require.LessOrEqual(t, end, len(sourceRangeArticle))
	}
	// the front matter table, raw HTML and the rule get none; the nested item gets its own
	assert.Equal(t, []string{"h1", "p", "li", "li", "li", "blockquote", "pre", "table", "h2", "p"}, tags)

	blocks := markdown.SourceBlocks(t.Context(), []byte(sourceRangeArticle))
	require.Len(t, blocks, len(matches), "SourceBlocks finds the ranges the renderer writes")
	for i, b := range blocks {
		assert.Equal(t, matches[i][2]+"-"+matches[i][3], strconv.Itoa(b.Start)+"-"+strconv.Itoa(b.End))
	}

	assert.Equal(t, "heading", blocks[0].Kind)
	assert.Equal(t, "Açaí 🍓", sourceRangeArticle[blocks[0].Start:blocks[0].End], "byte offsets, front matter included")
	assert.Equal(t, "O açaí é popular — 東京では人気です。", blocks[1].Quote)
	assert.Equal(t, blocks[0].SectionAnchor, blocks[1].SectionAnchor)
	assert.NotEmpty(t, blocks[0].SectionAnchor)
	assert.Equal(t, "citação segunda", blocks[5].Quote, "rendered text, not the raw slice with its > markers")
	assert.Equal(t, `fmt.Println("olá")`, blocks[6].Quote)
	assert.NotEqual(t, blocks[0].SectionAnchor, blocks[9].SectionAnchor, "after ## Fim")
	assert.Equal(t, "Último link e negrito.", blocks[9].Quote)
}

func TestSourceRangesCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(sourceRangeArticle, "\n", "\r\n")
	blocks := markdown.SourceBlocks(t.Context(), []byte(crlf))
	lf := markdown.SourceBlocks(t.Context(), []byte(sourceRangeArticle))
	assert.Equal(t, lf, blocks, "offsets count the README after end-of-line normalisation")
}

func TestVerifySourceRange(t *testing.T) {
	raw := []byte(sourceRangeArticle)
	blocks := markdown.SourceBlocks(t.Context(), raw)
	b, ok := markdown.VerifySourceRange(t.Context(), raw, blocks[1].Start, blocks[1].End)
	require.True(t, ok)
	assert.Equal(t, blocks[1], *b)

	for _, r := range [][2]int{{blocks[1].Start, blocks[1].End - 1}, {0, 3}, {-1, 5}, {5, 5}, {0, len(raw) + 1}} {
		_, ok := markdown.VerifySourceRange(t.Context(), raw, r[0], r[1])
		assert.False(t, ok, "range %v", r)
	}
}

func TestParseSourceRange(t *testing.T) {
	s, e, ok := markdown.ParseSourceRange("12-40")
	assert.True(t, ok)
	assert.Equal(t, 12, s)
	assert.Equal(t, 40, e)
	for _, bad := range []string{"", "12", "12-", "-40", "40-12", "12-12", "012-40", "12-40x", " 12-40", "1e3-2000", "+1-2"} {
		_, _, ok := markdown.ParseSourceRange(bad)
		assert.False(t, ok, "%q", bad)
	}
}

func TestSourceRangeSanitizer(t *testing.T) {
	// an author's attribute syntax can't put a value of their own on a heading
	out := renderWithSourceRanges(t, "# Title {data-source-range=\"1-2\"}\n", true)
	assert.Contains(t, out, `data-source-range="2-8"`, "the heading line, up to the attribute block")
	assert.NotContains(t, out, `data-source-range="1-2"`)

	// raw HTML keeps a well-formed range (the server re-checks it) and loses anything else
	out = renderWithSourceRanges(t, "<p data-source-range=\"3-9\">a</p>\n\n<p data-source-range=\"x\">b</p>\n\n<span data-source-range=\"3-9\">c</span>\n", false)
	assert.Contains(t, out, `<p data-source-range="3-9">a</p>`)
	assert.Contains(t, out, `<p>b</p>`)
	assert.NotContains(t, out, `<span data-source-range`)
}
