// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package markdown

import (
	"bytes"

	"code.gitea.io/gitea/modules/markup"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// internalLinkParser parses the Forkana internal link shorthand whose destination has spaces
// (issue #399):
//
//	[Solar System](subject/Solar System)
//	[Solar System](subject/Solar System/pierre)
//
// CommonMark does not allow spaces in a link destination (it would need
// "<subject/Solar System>" or "subject/Solar%20System"), so the standard link parser leaves
// that text as it is. Subject names are written with their spaces in articles, so this parser
// takes such a link when its destination starts with a Forkana page (see
// markup.ForkanaInternalLinkRoots), and leaves every other link to the standard parser. The
// rendered href is resolved to the instance root by markup.processForkanaLink.
//
// The label is taken as plain text: it cannot contain brackets or a line break, and inline
// markup inside it is not interpreted (the link-to-subject flow of the editor inserts the
// subject title). A destination may contain balanced parentheses, as in
// "subject/Mercury (planet)", but no line break, "<" or ">".
type internalLinkParser struct{}

// NewInternalLinkParser returns the parser of internal links with spaces.
func NewInternalLinkParser() parser.InlineParser {
	return &internalLinkParser{}
}

func (p *internalLinkParser) Trigger() []byte {
	return []byte{'['}
}

// parseInternalLinkShorthand returns the label and destination of an internal link with
// spaces at the start of line, and the number of bytes it spans; ok is false when line does
// not start with one (the standard link parser then handles it).
func parseInternalLinkShorthand(line []byte) (label, dest []byte, n int, ok bool) {
	if len(line) == 0 || line[0] != '[' {
		return nil, nil, 0, false
	}
	closeLabel := -1
	for i := 1; i < len(line); i++ {
		c := line[i]
		if c == '\\' {
			i++ // an escaped character is part of the label
			continue
		}
		if c == '[' || c == '\n' || c == '\r' {
			return nil, nil, 0, false
		}
		if c == ']' {
			closeLabel = i
			break
		}
	}
	if closeLabel < 2 || closeLabel+1 >= len(line) || line[closeLabel+1] != '(' {
		return nil, nil, 0, false
	}
	start := closeLabel + 2
	depth := 0
	end := -1
	for i := start; i < len(line) && end < 0; i++ {
		switch line[i] {
		case '\n', '\r', '<', '>':
			return nil, nil, 0, false
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			if depth == 0 {
				end = i
			} else {
				depth--
			}
		}
	}
	if end < 0 {
		return nil, nil, 0, false
	}
	dest = bytes.TrimSpace(line[start:end])
	// Only take what the standard parser cannot: a destination with a space.
	if !bytes.ContainsRune(dest, ' ') {
		return nil, nil, 0, false
	}
	if _, isInternal := markup.ParseForkanaInternalLink(string(dest)); !isInternal {
		return nil, nil, 0, false
	}
	return line[1:closeLabel], dest, end + 1, true
}

func (p *internalLinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	label, dest, n, ok := parseInternalLinkShorthand(line)
	if !ok {
		return nil
	}
	link := ast.NewLink()
	link.Destination = append([]byte(nil), dest...)
	link.AppendChild(link, ast.NewString(util.UnescapePunctuations(append([]byte(nil), label...))))
	block.Advance(n)
	return link
}
