// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"code.gitea.io/gitea/modules/markup"
	giteautil "code.gitea.io/gitea/modules/util"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// SourceRangeAttr is the attribute that carries a block's source range, "start-end", when
// RenderOptions.SourceRanges is set. Moderation reports anchor to it.
//
// The offsets are bytes into the README after end-of-line normalisation, front matter
// included, end exclusive. They are bytes, not characters: the browser must send back the
// attribute value it read, and never compute offsets from the DOM text (which is UTF-16).
const SourceRangeAttr = "data-source-range"

// MaxSourceBlockQuote caps the rendered text kept for a block, in runes.
const MaxSourceBlockQuote = 500

// SourceBlock is a block of an article that a report can anchor to.
type SourceBlock struct {
	Start, End int
	// Kind is "paragraph", "heading", "list_item", "blockquote", "code", "table" or "definition_list".
	Kind string
	// Quote is the block's rendered text (whitespace collapsed, capped at MaxSourceBlockQuote runes).
	Quote string
	// SectionAnchor is the id of the nearest heading before the block (the block's own id for a
	// heading), or "" before the first heading.
	SectionAnchor string
}

// sourceBlockKind tells whether a node gets a source range, and what kind of block it is.
//
// List items get one wherever they are (a nested item gives a nested range; the innermost is
// the anchor). The other blocks only get one at the top level: a paragraph in a list item or a
// blockquote is part of that block. Raw HTML, rules and indented code blocks get none (their
// HTML can't carry the attribute), so reports on them are quote-only.
func sourceBlockKind(n ast.Node) (string, bool) {
	switch n.(type) {
	case *ast.ListItem, *TaskCheckBoxListItem:
		return "list_item", true
	}
	if _, top := n.Parent().(*ast.Document); !top {
		return "", false
	}
	switch n.(type) {
	case *ast.Paragraph:
		return "paragraph", true
	case *ast.Heading:
		return "heading", true
	case *ast.Blockquote:
		return "blockquote", true
	case *ast.FencedCodeBlock:
		return "code", true
	case *east.Table:
		return "table", true
	case *east.DefinitionList:
		return "definition_list", true
	}
	return "", false
}

// nodeSourceRange returns the byte span, in the parsed body, of the node's own lines and of
// every line and text segment below it. Nodes Forkana created (the front matter table, the
// table of contents) have no segments and get none.
func nodeSourceRange(n ast.Node) (start, end int, ok bool) {
	start, end = -1, -1
	add := func(s, e int) {
		if s >= e {
			return
		}
		if start < 0 || s < start {
			start = s
		}
		if e > end {
			end = e
		}
	}
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if c.Type() == ast.TypeBlock {
			lines := c.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				add(seg.Start, seg.Stop)
			}
		}
		if t, isText := c.(*ast.Text); isText {
			add(t.Segment.Start, t.Segment.Stop)
		}
		return ast.WalkContinue, nil
	})
	return start, end, start >= 0
}

// walkSourceBlocks calls fn for every block that gets a source range, in document order, with
// the range shifted by metaLength so that it counts from the start of the whole README.
func walkSourceBlocks(doc ast.Node, metaLength int, fn func(n ast.Node, kind string, start, end int)) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n == doc {
			return ast.WalkContinue, nil
		}
		kind, ok := sourceBlockKind(n)
		if !ok {
			return ast.WalkContinue, nil
		}
		if start, end, ok := nodeSourceRange(n); ok {
			fn(n, kind, metaLength+start, metaLength+end)
		}
		return ast.WalkContinue, nil
	})
}

// annotateSourceRanges sets the source range attribute on every block that gets one.
func annotateSourceRanges(doc *ast.Document, rc *RenderConfig) {
	walkSourceBlocks(doc, rc.metaLength, func(n ast.Node, _ string, start, end int) {
		// SetAttributeString replaces a value the author may have written with the
		// attribute syntax, so the attribute only ever holds the renderer's own range.
		n.SetAttributeString(SourceRangeAttr, fmt.Appendf(nil, "%d-%d", start, end))
	})
}

// blockQuote returns the rendered text of a block: its text segments, with whitespace
// collapsed, capped at MaxSourceBlockQuote runes.
func blockQuote(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.FencedCodeBlock:
			lines := t.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.Write(seg.Value(source))
			}
		}
		if c.Type() == ast.TypeBlock && c != n {
			b.WriteByte(' ')
		}
		return ast.WalkContinue, nil
	})
	quote := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(quote) > MaxSourceBlockQuote {
		quote = string([]rune(quote)[:MaxSourceBlockQuote])
	}
	return quote
}

// SourceBlocks parses an article README the way the renderer does and returns the blocks a
// report can anchor to, in document order, with the same ranges the renderer writes.
func SourceBlocks(ctx context.Context, raw []byte) []SourceBlock {
	full := giteautil.NormalizeEOL(raw)
	rc := &RenderConfig{}
	body, _ := ExtractMetadataBytes(full, rc)
	rc.metaLength = len(full) - len(body)

	rctx := markup.NewRenderContext(ctx)
	pc := newParserContext(rctx)
	pc.Set(renderConfigKey, rc)
	doc := SpecializedMarkdown(rctx).goldmarkMarkdown.Parser().Parse(text.NewReader(body), parser.WithContext(pc))

	var blocks []SourceBlock
	section := ""
	walkSourceBlocks(doc, rc.metaLength, func(n ast.Node, kind string, start, end int) {
		if h, isHeading := n.(*ast.Heading); isHeading {
			if id, found := h.AttributeString("id"); found {
				if b, isBytes := id.([]byte); isBytes {
					section = string(b)
				}
			}
		}
		blocks = append(blocks, SourceBlock{
			Start:         start,
			End:           end,
			Kind:          kind,
			Quote:         blockQuote(n, body),
			SectionAnchor: section,
		})
	})
	return blocks
}

// VerifySourceRange checks that start-end is the range of a block of the README, and returns
// that block. The range a browser sends is never trusted: this is how the server learns what
// was reported.
func VerifySourceRange(ctx context.Context, raw []byte, start, end int) (*SourceBlock, bool) {
	if start < 0 || end <= start || end > len(raw) {
		return nil, false
	}
	for _, b := range SourceBlocks(ctx, raw) {
		if b.Start == start && b.End == end {
			return &b, true
		}
	}
	return nil, false
}

// ParseSourceRange parses a "start-end" attribute value.
func ParseSourceRange(s string) (start, end int, ok bool) {
	before, after, found := strings.Cut(s, "-")
	if !found {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(before+" "+after, "%d %d", &start, &end); err != nil {
		return 0, 0, false
	}
	if fmt.Sprintf("%d-%d", start, end) != s || start < 0 || end <= start {
		return 0, 0, false
	}
	return start, end, true
}
