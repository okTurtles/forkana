// Copyright 2019 Yusuke Inuzuka
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Most of what follows is a subtly changed version of github.com/yuin/goldmark/extension/footnote.go

package common

import (
	"bytes"
	"fmt"
	"html/template"
	"strconv"
	"unicode"

	"code.gitea.io/gitea/modules/htmlutil"
	"code.gitea.io/gitea/modules/markup/internal"
	"code.gitea.io/gitea/modules/translation"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// CleanValue will clean a value to make it safe to be an id
// This function is quite different from the original goldmark function
// and more closely matches the output from the shurcooL sanitizer
// In particular Unicode letters and numbers are a lot more than a-zA-Z0-9...
func CleanValue(value []byte) []byte {
	value = bytes.TrimSpace(value)
	rs := bytes.Runes(value)
	result := make([]rune, 0, len(rs))
	for _, r := range rs {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-' {
			result = append(result, unicode.ToLower(r))
		}
		if unicode.IsSpace(r) {
			result = append(result, '-')
		}
	}
	return []byte(string(result))
}

// Most of what follows is a subtly changed version of github.com/yuin/goldmark/extension/footnote.go

// A FootnoteLink struct represents a link to a footnote of Markdown
// (PHP Markdown Extra) text.
type FootnoteLink struct {
	ast.BaseInline
	Index int
	Name  []byte
	// RefIndex is which use of the footnote this is (1 for the first): a source cited
	// several times gets one back-link per use in the reference list (Forkana, issue #399).
	RefIndex int
}

// Dump implements Node.Dump.
func (n *FootnoteLink) Dump(source []byte, level int) {
	m := map[string]string{}
	m["Index"] = strconv.Itoa(n.Index)
	m["Name"] = fmt.Sprintf("%v", n.Name)
	m["RefIndex"] = strconv.Itoa(n.RefIndex)
	ast.DumpHelper(n, source, level, m, nil)
}

// KindFootnoteLink is a NodeKind of the FootnoteLink node.
var KindFootnoteLink = ast.NewNodeKind("GiteaFootnoteLink")

// Kind implements Node.Kind.
func (n *FootnoteLink) Kind() ast.NodeKind {
	return KindFootnoteLink
}

// NewFootnoteLink returns a new FootnoteLink node for the refIndex-th use of a footnote.
func NewFootnoteLink(index int, name []byte, refIndex int) *FootnoteLink {
	return &FootnoteLink{
		Index:    index,
		Name:     name,
		RefIndex: refIndex,
	}
}

// A FootnoteBackLink struct represents a link to a footnote of Markdown
// (PHP Markdown Extra) text.
type FootnoteBackLink struct {
	ast.BaseInline
	Index int
	Name  []byte
	// RefCount is how many times the footnote is used in the text: one back-link per use.
	RefCount int
}

// Dump implements Node.Dump.
func (n *FootnoteBackLink) Dump(source []byte, level int) {
	m := map[string]string{}
	m["Index"] = strconv.Itoa(n.Index)
	m["Name"] = fmt.Sprintf("%v", n.Name)
	m["RefCount"] = strconv.Itoa(n.RefCount)
	ast.DumpHelper(n, source, level, m, nil)
}

// KindFootnoteBackLink is a NodeKind of the FootnoteBackLink node.
var KindFootnoteBackLink = ast.NewNodeKind("GiteaFootnoteBackLink")

// Kind implements Node.Kind.
func (n *FootnoteBackLink) Kind() ast.NodeKind {
	return KindFootnoteBackLink
}

// NewFootnoteBackLink returns a new FootnoteBackLink node for a footnote used refCount times.
func NewFootnoteBackLink(index int, name []byte, refCount int) *FootnoteBackLink {
	return &FootnoteBackLink{
		Index:    index,
		Name:     name,
		RefCount: refCount,
	}
}

// A Footnote struct represents a footnote of Markdown
// (PHP Markdown Extra) text.
type Footnote struct {
	ast.BaseBlock
	Ref   []byte
	Index int
	Name  []byte
	// RefCount is how many times the footnote is used in the text.
	RefCount int
}

// Dump implements Node.Dump.
func (n *Footnote) Dump(source []byte, level int) {
	m := map[string]string{}
	m["Index"] = strconv.Itoa(n.Index)
	m["Ref"] = string(n.Ref)
	m["Name"] = string(n.Name)
	m["RefCount"] = strconv.Itoa(n.RefCount)
	ast.DumpHelper(n, source, level, m, nil)
}

// KindFootnote is a NodeKind of the Footnote node.
var KindFootnote = ast.NewNodeKind("GiteaFootnote")

// Kind implements Node.Kind.
func (n *Footnote) Kind() ast.NodeKind {
	return KindFootnote
}

// NewFootnote returns a new Footnote node.
func NewFootnote(ref []byte) *Footnote {
	return &Footnote{
		Ref:   ref,
		Index: -1,
		Name:  ref,
	}
}

// A FootnoteList struct represents footnotes of Markdown
// (PHP Markdown Extra) text.
type FootnoteList struct {
	ast.BaseBlock
	Count int
	// backToText is the back-links' label in the document's language, resolved once when the
	// list is rendered (before its entries).
	backToText string
}

// Dump implements Node.Dump.
func (n *FootnoteList) Dump(source []byte, level int) {
	m := map[string]string{}
	m["Count"] = strconv.Itoa(n.Count)
	ast.DumpHelper(n, source, level, m, nil)
}

// KindFootnoteList is a NodeKind of the FootnoteList node.
var KindFootnoteList = ast.NewNodeKind("GiteaFootnoteList")

// Kind implements Node.Kind.
func (n *FootnoteList) Kind() ast.NodeKind {
	return KindFootnoteList
}

// NewFootnoteList returns a new FootnoteList node.
func NewFootnoteList() *FootnoteList {
	return &FootnoteList{
		Count: 0,
	}
}

var footnoteListKey = parser.NewContextKey()

type footnoteBlockParser struct{}

var defaultFootnoteBlockParser = &footnoteBlockParser{}

// NewFootnoteBlockParser returns a new parser.BlockParser that can parse
// footnotes of the Markdown(PHP Markdown Extra) text.
func NewFootnoteBlockParser() parser.BlockParser {
	return defaultFootnoteBlockParser
}

func (b *footnoteBlockParser) Trigger() []byte {
	return []byte{'['}
}

func (b *footnoteBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || line[pos] != '[' {
		return nil, parser.NoChildren
	}
	pos++
	if pos > len(line)-1 || line[pos] != '^' {
		return nil, parser.NoChildren
	}
	open := pos + 1
	closure := util.FindClosure(line[pos+1:], '[', ']', false, false) //nolint:staticcheck // deprecated function
	closes := pos + 1 + closure
	next := closes + 1
	if closure > -1 {
		if next >= len(line) || line[next] != ':' {
			return nil, parser.NoChildren
		}
	} else {
		return nil, parser.NoChildren
	}
	padding := segment.Padding
	label := reader.Value(text.NewSegment(segment.Start+open-padding, segment.Start+closes-padding))
	if util.IsBlank(label) {
		return nil, parser.NoChildren
	}
	item := NewFootnote(label)

	pos = next + 1 - padding
	if pos >= len(line) {
		reader.Advance(pos)
		return item, parser.NoChildren
	}
	reader.AdvanceAndSetPadding(pos, padding)
	return item, parser.HasChildren
}

func (b *footnoteBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if util.IsBlank(line) {
		return parser.Continue | parser.HasChildren
	}
	childpos, padding := util.IndentPosition(line, reader.LineOffset(), 4)
	if childpos < 0 {
		return parser.Close
	}
	reader.AdvanceAndSetPadding(childpos, padding)
	return parser.Continue | parser.HasChildren
}

func (b *footnoteBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	var list *FootnoteList
	if tlist := pc.Get(footnoteListKey); tlist != nil {
		list = tlist.(*FootnoteList)
	} else {
		list = NewFootnoteList()
		pc.Set(footnoteListKey, list)
		node.Parent().InsertBefore(node.Parent(), node, list)
	}
	node.Parent().RemoveChild(node.Parent(), node)
	list.AppendChild(list, node)
}

func (b *footnoteBlockParser) CanInterruptParagraph() bool {
	return true
}

func (b *footnoteBlockParser) CanAcceptIndentedLine() bool {
	return false
}

type footnoteParser struct{}

var defaultFootnoteParser = &footnoteParser{}

// NewFootnoteParser returns a new parser.InlineParser that can parse
// footnote links of the Markdown(PHP Markdown Extra) text.
func NewFootnoteParser() parser.InlineParser {
	return defaultFootnoteParser
}

func (s *footnoteParser) Trigger() []byte {
	// footnote syntax probably conflict with the image syntax.
	// So we need trigger this parser with '!'.
	return []byte{'!', '['}
}

func (s *footnoteParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, segment := block.PeekLine()
	pos := 1
	if len(line) > 0 && line[0] == '!' {
		pos++
	}
	if pos >= len(line) || line[pos] != '^' {
		return nil
	}
	pos++
	if pos >= len(line) {
		return nil
	}
	open := pos
	closure := util.FindClosure(line[pos:], '[', ']', false, false) //nolint:staticcheck // deprecated function
	if closure < 0 {
		return nil
	}
	closes := pos + closure
	value := block.Value(text.NewSegment(segment.Start+open, segment.Start+closes))
	block.Advance(closes + 1)

	var list *FootnoteList
	if tlist := pc.Get(footnoteListKey); tlist != nil {
		list = tlist.(*FootnoteList)
	}
	if list == nil {
		return nil
	}
	index := 0
	refIndex := 0
	name := []byte{}
	for def := list.FirstChild(); def != nil; def = def.NextSibling() {
		d := def.(*Footnote)
		if bytes.Equal(d.Ref, value) {
			if d.Index < 0 {
				list.Count++
				d.Index = list.Count
				val := CleanValue(d.Name)
				if len(val) == 0 {
					val = []byte(strconv.Itoa(d.Index))
				}
				d.Name = pc.IDs().Generate(val, KindFootnote)
			}
			d.RefCount++
			index = d.Index
			refIndex = d.RefCount
			name = d.Name
			break
		}
	}
	if index == 0 {
		return nil
	}

	return NewFootnoteLink(index, name, refIndex)
}

type footnoteASTTransformer struct{}

var defaultFootnoteASTTransformer = &footnoteASTTransformer{}

// NewFootnoteASTTransformer returns a new parser.ASTTransformer that
// insert a footnote list to the last of the document.
func NewFootnoteASTTransformer() parser.ASTTransformer {
	return defaultFootnoteASTTransformer
}

func (a *footnoteASTTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	var list *FootnoteList
	if tlist := pc.Get(footnoteListKey); tlist != nil {
		list = tlist.(*FootnoteList)
	} else {
		return
	}
	pc.Set(footnoteListKey, nil)
	for footnote := list.FirstChild(); footnote != nil; {
		// Forkana (issue #399): the back-links ("^", or "^ a b" for a source cited several
		// times) open the reference, as on Wikipedia, so they go at the start of its first
		// paragraph rather than at the end of its last one.
		container := footnote
		next := footnote.NextSibling()
		if fc := container.FirstChild(); fc != nil && ast.IsParagraph(fc) {
			container = fc
		}
		footnoteNode := footnote.(*Footnote)
		index := footnoteNode.Index
		name := footnoteNode.Name
		if index < 0 {
			list.RemoveChild(list, footnote)
		} else {
			backLink := NewFootnoteBackLink(index, name, footnoteNode.RefCount)
			if fc := container.FirstChild(); fc != nil {
				container.InsertBefore(container, fc, backLink)
			} else {
				// an empty definition (InsertBefore would count the child twice)
				container.AppendChild(container, backLink)
			}
		}
		footnote = next
	}
	list.SortChildren(func(n1, n2 ast.Node) int {
		if n1.(*Footnote).Index < n2.(*Footnote).Index {
			return -1
		}
		return 1
	})
	if list.Count <= 0 {
		list.Parent().RemoveChild(list.Parent(), list)
		return
	}

	node.AppendChild(node, list)
}

// FootnoteHTMLRenderer is a renderer.NodeRenderer implementation that
// renders FootnoteLink nodes.
//
// Forkana (issue #399) renders footnotes as numbered references, the way Wikipedia does:
// a superscript "[1]" in the text, and a "References" list at the bottom of the document
// whose entries open with a "^" back-link, or with "^ a b" when the source is cited more
// than once (each letter returns to a different place in the text).
type FootnoteHTMLRenderer struct {
	html.Config
	renderInternal *internal.RenderInternal
}

// NewFootnoteHTMLRenderer returns a new FootnoteHTMLRenderer. renderInternal may be nil, the
// classes are then written as they are (and dropped by the sanitizer).
func NewFootnoteHTMLRenderer(renderInternal *internal.RenderInternal, opts ...html.Option) renderer.NodeRenderer {
	r := &FootnoteHTMLRenderer{
		Config:         html.NewConfig(),
		renderInternal: renderInternal,
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *FootnoteHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindFootnoteLink, r.renderFootnoteLink)
	reg.Register(KindFootnoteBackLink, r.renderFootnoteBackLink)
	reg.Register(KindFootnote, r.renderFootnote)
	reg.Register(KindFootnoteList, r.renderFootnoteList)
}

// writeHTML writes HTML built by the renderer itself, keeping its "class" attributes through
// the sanitizer (each tag may carry one).
func (r *FootnoteHTMLRenderer) writeHTML(w util.BufWriter, s template.HTML) {
	if r.renderInternal != nil {
		s = r.renderInternal.ProtectSafeAttrs(s)
	}
	_, _ = w.WriteString(string(s))
}

// FootnoteRefID is the id of the refIndex-th use (1-based) of the footnote named name in the
// text, the target of its back-link. The first use keeps the historical "fnref:<name>"; the
// following ones add ":<n>", a character CleanValue never puts in a name.
func FootnoteRefID(name []byte, refIndex int) string {
	if refIndex <= 1 {
		return "fnref:" + string(name)
	}
	return "fnref:" + string(name) + ":" + strconv.Itoa(refIndex)
}

// FootnoteBackLinkLetter is the letter of the n-th use (1-based) of a source cited several
// times: a, b, …, z, aa, ab, … (Wikipedia's "^ a b").
func FootnoteBackLinkLetter(n int) string {
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('a' + n%26)}, b...)
		n /= 26
	}
	return string(b)
}

// documentLocale is the locale of the document the node is in: the language of its front
// matter if it has one, else the default language (like the table of contents' title).
func documentLocale(node ast.Node) translation.Locale {
	for n := node; n != nil; n = n.Parent() {
		if n.Kind() != ast.KindDocument {
			continue
		}
		if lang, ok := n.AttributeString("lang"); ok {
			switch v := lang.(type) {
			case []byte:
				return translation.NewLocale(string(v))
			case string:
				return translation.NewLocale(v)
			}
		}
		break
	}
	return translation.NewLocale("") // the default language, as for the table of contents
}

func (r *FootnoteHTMLRenderer) renderFootnoteLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*FootnoteLink)
		r.writeHTML(w, htmlutil.HTMLFormat(`<sup class="footnote-ref" id="%s"><a href="#fn:%s" role="doc-noteref">[%d]</a></sup>`,
			FootnoteRefID(n.Name, n.RefIndex), string(n.Name), n.Index))
	}
	return ast.WalkContinue, nil
}

func (r *FootnoteHTMLRenderer) renderFootnoteBackLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*FootnoteBackLink)
	label := ""
	for p := node.Parent(); p != nil; p = p.Parent() {
		if list, ok := p.(*FootnoteList); ok {
			label = list.backToText
			break
		}
	}
	if label == "" {
		label = documentLocale(node).TrString("citation.back_to_text")
	}
	r.writeHTML(w, `<span class="footnote-backrefs">`)
	if n.RefCount <= 1 {
		r.writeHTML(w, htmlutil.HTMLFormat(`<a href="#%s" class="footnote-backref" role="doc-backlink" aria-label="%s">^</a>`,
			FootnoteRefID(n.Name, 1), label))
	} else {
		// Several uses: "^" is not a link, each letter is.
		_, _ = w.WriteString("^")
		for i := 1; i <= n.RefCount; i++ {
			letter := FootnoteBackLinkLetter(i)
			r.writeHTML(w, htmlutil.HTMLFormat(` <a href="#%s" class="footnote-backref" role="doc-backlink" aria-label="%s">%s</a>`,
				FootnoteRefID(n.Name, i), label+" "+letter, letter))
		}
	}
	_, _ = w.WriteString(`</span> `)
	return ast.WalkContinue, nil
}

func (r *FootnoteHTMLRenderer) renderFootnote(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Footnote)
	if entering {
		_, _ = w.WriteString(`<li id="fn:`)
		_, _ = w.Write(n.Name)
		_, _ = w.WriteString(`" role="doc-endnote"`)
		if node.Attributes() != nil {
			html.RenderAttributes(w, node, html.ListItemAttributeFilter)
		}
		_, _ = w.WriteString(">\n")
	} else {
		_, _ = w.WriteString("</li>\n")
	}
	return ast.WalkContinue, nil
}

func (r *FootnoteHTMLRenderer) renderFootnoteList(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		locale := documentLocale(node)
		node.(*FootnoteList).backToText = locale.TrString("citation.back_to_text")
		r.writeHTML(w, `<div class="footnotes" role="doc-endnotes">`)
		r.writeHTML(w, htmlutil.HTMLFormat("\n<h2 class=\"footnotes-heading\">%s</h2>\n",
			locale.TrString("citation.references")))
		_, _ = w.WriteString("<ol>\n")
	} else {
		_, _ = w.WriteString("</ol>\n")
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

type footnoteExtension struct {
	renderInternal *internal.RenderInternal
}

// FootnoteExtension represents the Gitea Footnote, for the users whose output keeps no
// classes (see NewFootnoteExtension).
var FootnoteExtension = &footnoteExtension{}

// NewFootnoteExtension returns the Gitea Footnote extension, whose rendered classes
// ("footnote-ref", "footnotes", …) renderInternal keeps through the sanitizer.
func NewFootnoteExtension(renderInternal *internal.RenderInternal) goldmark.Extender {
	return &footnoteExtension{renderInternal: renderInternal}
}

// Extend extends the markdown converter with the Gitea Footnote parser
func (e *footnoteExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithBlockParsers(
			util.Prioritized(NewFootnoteBlockParser(), 999),
		),
		parser.WithInlineParsers(
			util.Prioritized(NewFootnoteParser(), 101),
		),
		parser.WithASTTransformers(
			util.Prioritized(NewFootnoteASTTransformer(), 999),
		),
	)
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(NewFootnoteHTMLRenderer(e.renderInternal), 500),
	))
}
