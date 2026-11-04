// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"slices"
	"strings"

	"code.gitea.io/gitea/modules/markup/common"

	"golang.org/x/net/html"
)

func isAnchorIDUserContent(s string) bool {
	// blackfridayExtRegex is for blackfriday extensions create IDs like fn:user-content-footnote
	// old logic: blackfridayExtRegex = regexp.MustCompile(`[^:]*:user-content-`)
	return strings.HasPrefix(s, "user-content-") || strings.Contains(s, ":user-content-")
}

func isAnchorIDFootnote(s string) bool {
	return strings.HasPrefix(s, "fnref:user-content-") || strings.HasPrefix(s, "fn:user-content-")
}

func isAnchorHrefFootnote(s string) bool {
	return strings.HasPrefix(s, "#fnref:user-content-") || strings.HasPrefix(s, "#fn:user-content-")
}

func isHeadingTag(node *html.Node) bool {
	return len(node.Data) == 2 && node.Data[0] == 'h' && node.Data[1] >= '1' && node.Data[1] <= '6'
}

// getNodeText returns the text of the node and all its descendants
func getNodeText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var sb strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		sb.WriteString(getNodeText(child))
	}
	return sb.String()
}

func processNodeAttrID(ctx *RenderContext, node *html.Node) {
	// Add user-content- to IDs and "#" links if they don't already have them,
	// and convert the link href to a relative link to the host root
	hasID := false
	for idx, attr := range node.Attr {
		if attr.Key == "id" {
			hasID = true
			if !isAnchorIDUserContent(attr.Val) {
				node.Attr[idx].Val = "user-content-" + attr.Val
			}
		}
	}

	if hasID || !ctx.RenderOptions.EnableHeadingIDGeneration || !isHeadingTag(node) {
		return
	}
	// The footnotes "References" heading has nothing to link to, and an ID would clash with
	// a "References" heading written by the user.
	for _, attr := range node.Attr {
		if attr.Key == "class" && slices.Contains(strings.Fields(attr.Val), "footnotes-heading") {
			return
		}
	}
	// Raw HTML headings (e.g. "<h2>History</h2>" in Wikipedia imports) get an ID from their text,
	// like Markdown headings do, so they get a permalink. Like upstream, duplicates aren't
	// deduplicated: the first heading wins when following a link.
	if id := string(common.CleanValue([]byte(getNodeText(node)))); id != "" {
		node.Attr = append(node.Attr, html.Attribute{Key: "id", Val: "user-content-" + id})
	}
}

func processFootnoteNode(ctx *RenderContext, node *html.Node) {
	for idx, attr := range node.Attr {
		if (attr.Key == "id" && isAnchorIDFootnote(attr.Val)) ||
			(attr.Key == "href" && isAnchorHrefFootnote(attr.Val)) {
			if footnoteContextID := ctx.RenderOptions.Metas["footnoteContextId"]; footnoteContextID != "" {
				node.Attr[idx].Val = attr.Val + "-" + footnoteContextID
			}
			continue
		}
	}
}

func processNodeA(ctx *RenderContext, node *html.Node) {
	for idx, attr := range node.Attr {
		if attr.Key == "href" {
			if anchorID, ok := strings.CutPrefix(attr.Val, "#"); ok {
				if !isAnchorIDUserContent(attr.Val) {
					node.Attr[idx].Val = "#user-content-" + anchorID
				}
			} else if !processForkanaLink(ctx, node, idx, attr.Val) {
				node.Attr[idx].Val = ctx.RenderHelper.ResolveLink(attr.Val, LinkTypeDefault)
			}
		}
	}
}

func visitNodeImg(ctx *RenderContext, img *html.Node) (next *html.Node) {
	next = img.NextSibling
	attrSrc, hasLazy := "", false
	for i, imgAttr := range img.Attr {
		hasLazy = hasLazy || imgAttr.Key == "loading" && imgAttr.Val == "lazy"
		if imgAttr.Key != "src" {
			attrSrc = imgAttr.Val
			continue
		}

		imgSrcOrigin := imgAttr.Val
		isLinkable := imgSrcOrigin != "" && !strings.HasPrefix(imgSrcOrigin, "data:")

		// By default, the "<img>" tag should also be clickable,
		// because frontend uses `<img>` to paste the re-scaled image into the Markdown,
		// so it must match the default Markdown image behavior.
		cnt := 0
		for p := img.Parent; isLinkable && p != nil && cnt < 2; p = p.Parent {
			if hasParentAnchor := p.Type == html.ElementNode && p.Data == "a"; hasParentAnchor {
				isLinkable = false
				break
			}
			cnt++
		}
		if isLinkable {
			wrapper := &html.Node{Type: html.ElementNode, Data: "a", Attr: []html.Attribute{
				{Key: "href", Val: ctx.RenderHelper.ResolveLink(imgSrcOrigin, LinkTypeDefault)},
				{Key: "target", Val: "_blank"},
			}}
			parent := img.Parent
			imgNext := img.NextSibling
			parent.RemoveChild(img)
			parent.InsertBefore(wrapper, imgNext)
			wrapper.AppendChild(img)
		}

		imgAttr.Val = ctx.RenderHelper.ResolveLink(imgSrcOrigin, LinkTypeMedia)
		imgAttr.Val = camoHandleLink(imgAttr.Val)
		img.Attr[i] = imgAttr
	}
	if !RenderBehaviorForTesting.DisableAdditionalAttributes && !hasLazy && !strings.HasPrefix(attrSrc, "data:") {
		img.Attr = append(img.Attr, html.Attribute{Key: "loading", Val: "lazy"})
	}
	return next
}

func visitNodeVideo(ctx *RenderContext, node *html.Node) (next *html.Node) {
	next = node.NextSibling
	for i, attr := range node.Attr {
		if attr.Key != "src" {
			continue
		}
		if IsNonEmptyRelativePath(attr.Val) {
			attr.Val = ctx.RenderHelper.ResolveLink(attr.Val, LinkTypeMedia)
		}
		attr.Val = camoHandleLink(attr.Val)
		node.Attr[i] = attr
	}
	return next
}
