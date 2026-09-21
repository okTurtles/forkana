// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

// Package attachmentref finds local attachment references in article content.
//
// It deliberately works on URLs rather than on Markdown or HTML syntax, so the
// same extractor serves Markdown images, ordinary links, `<img>`, `<video>`,
// `<audio>` and plain text alike.
package attachmentref

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/setting"
)

// MaxScanSize caps how much of a blob is scanned, so history walks stay bounded.
// A reference truncated by this cap is simply not seen; the grace period before
// garbage collection plus add-only reconciliation cover that case.
const MaxScanSize = 1 << 20 // 1 MiB

const (
	marker   = "attachments/"
	uuidLen  = 36
	minSegs  = 2
	uuidPart = "attachments"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ExtractAttachmentUUIDs returns the lower-cased UUIDs of every local attachment
// reference in content, deduplicated and in order of first appearance.
//
// Accepted forms are the relative and root-relative attachment paths, the
// article-scoped and repository-scoped paths, and absolute URLs whose host and
// application subpath match the configured AppURL. External-domain lookalikes
// and malformed UUIDs are ignored.
//
// Retaining a false positive is preferable to missing a reference: the returned
// UUIDs are only candidates, and CanAssociate still decides whether the target
// repository may keep any of them alive.
func ExtractAttachmentUUIDs(content string) []string {
	if len(content) > MaxScanSize {
		content = strings.ToValidUTF8(content[:MaxScanSize], "")
	}
	// Binary content cannot hold a reference worth trusting, and scanning it
	// would only produce noise.
	if strings.IndexByte(content, 0) >= 0 || !utf8.ValidString(content) {
		return nil
	}

	var appHost string
	if appURL, err := url.Parse(setting.AppURL); err == nil {
		appHost = appURL.Host
	}

	var uuids []string
	seen := make(container.Set[string])
	for offset := 0; offset < len(content); {
		idx := strings.Index(content[offset:], marker)
		if idx < 0 {
			break
		}
		markerPos := offset + idx
		uuidPos := markerPos + len(marker)
		offset = uuidPos

		attachmentUUID, ok := uuidAt(content[uuidPos:])
		if !ok {
			continue
		}
		start := refStart(content, markerPos)
		if !isLocalAttachmentRef(content[start:uuidPos+uuidLen], appHost) {
			continue
		}
		if seen.Add(attachmentUUID) {
			uuids = append(uuids, attachmentUUID)
		}
	}
	return uuids
}

// uuidAt reads a well-formed UUID from the start of s, rejecting a longer
// token that merely begins with one.
func uuidAt(s string) (string, bool) {
	if len(s) < uuidLen {
		return "", false
	}
	candidate := s[:uuidLen]
	if !uuidPattern.MatchString(candidate) {
		return "", false
	}
	if len(s) > uuidLen && isTokenByte(s[uuidLen]) {
		return "", false
	}
	return strings.ToLower(candidate), true
}

// refStart walks back to the beginning of the URL token containing the marker,
// which is what tells a local path apart from an external lookalike.
func refStart(content string, markerPos int) int {
	start := markerPos
	for start > 0 && !isRefDelimiter(content[start-1]) {
		start--
	}
	return start
}

func isLocalAttachmentRef(ref, appHost string) bool {
	path := ref
	// A scheme-relative reference carries a host too, so it has to be checked
	// against AppURL just like an absolute one.
	if strings.Contains(ref, "://") || strings.HasPrefix(ref, "//") {
		refURL, err := url.Parse(ref)
		if err != nil || refURL.Host == "" || appHost == "" || !strings.EqualFold(refURL.Host, appHost) {
			return false
		}
		path = refURL.Path
	}

	if sub := setting.AppSubURL; sub != "" {
		switch {
		case strings.HasPrefix(path, sub+"/"):
			path = strings.TrimPrefix(path, sub)
		case strings.HasPrefix(path, "/"):
			// Root-relative, but outside this instance's mount point.
			return false
		}
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	return len(segments) >= minSegs && segments[len(segments)-minSegs] == uuidPart
}

// isRefDelimiter reports whether c cannot appear inside a URL reference as
// written in Markdown, HTML or plain text.
func isRefDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', '"', '\'', '`', '<', '>', '(', ')', '[', ']', '{', '}', '|', '\\', '*', '=', ',', ';':
		return true
	}
	return false
}

func isTokenByte(c byte) bool {
	return c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
