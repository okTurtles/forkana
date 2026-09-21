// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachmentref

import (
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
)

const (
	uuidA = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	uuidB = "b1ffcd88-8d1c-4fe9-cc7e-7cc8ce491b22"
)

func TestExtractAttachmentUUIDs(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/")()
	defer test.MockVariableValue(&setting.AppSubURL, "")()

	cases := []struct {
		name     string
		content  string
		expected []string
	}{
		{"empty", "", nil},
		{"markdown image root relative", "![a](/attachments/" + uuidA + ")", []string{uuidA}},
		{"markdown link relative", "[a](attachments/" + uuidA + ")", []string{uuidA}},
		{"legacy repository scoped", "![a](/user2/repo1/attachments/" + uuidA + ")", []string{uuidA}},
		{"article scoped", "![a](/user2/subject/article/attachments/" + uuidA + ")", []string{uuidA}},
		{"html img tag", `<img src="/attachments/` + uuidA + `" alt="a">`, []string{uuidA}},
		{"html video source", `<video><source src="/attachments/` + uuidA + `"></video>`, []string{uuidA}},
		{"html audio unquoted", `<audio src=/attachments/` + uuidA + `>`, []string{uuidA}},
		{"plain text", "see https://forkana.example/attachments/" + uuidA + " for details", []string{uuidA}},
		{"absolute app url", "![a](https://forkana.example/attachments/" + uuidA + ")", []string{uuidA}},
		{"query string", "![a](/attachments/" + uuidA + "?inline=1)", []string{uuidA}},
		{"fragment", "![a](/attachments/" + uuidA + "#top)", []string{uuidA}},
		{"upper case normalized", "![a](/attachments/" + strings.ToUpper(uuidA) + ")", []string{uuidA}},
		{
			"deduplicated in order of first appearance",
			"![a](/attachments/" + uuidB + ") ![b](/attachments/" + uuidA + ") ![c](/attachments/" + uuidB + ")",
			[]string{uuidB, uuidA},
		},
		{"external domain lookalike", "![a](https://evil.example/attachments/" + uuidA + ")", nil},
		{"protocol relative external", "![a](//evil.example/attachments/" + uuidA + ")", nil},
		{"malformed uuid too short", "![a](/attachments/a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a1)", nil},
		{"malformed uuid non hex", "![a](/attachments/z0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11)", nil},
		{"longer token starting with a uuid", "![a](/attachments/" + uuidA + "extra)", nil},
		{"marker without attachments segment", "![a](/somethingattachments/" + uuidA + ")", nil},
		{"binary content", "PNG\x00\x00" + "/attachments/" + uuidA, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, ExtractAttachmentUUIDs(c.content))
		})
	}
}

func TestExtractAttachmentUUIDsWithSubPath(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/sub/")()
	defer test.MockVariableValue(&setting.AppSubURL, "/sub")()

	cases := []struct {
		name     string
		content  string
		expected []string
	}{
		{"root relative within sub path", "![a](/sub/attachments/" + uuidA + ")", []string{uuidA}},
		{"absolute within sub path", "![a](https://forkana.example/sub/attachments/" + uuidA + ")", []string{uuidA}},
		{"root relative outside sub path", "![a](/attachments/" + uuidA + ")", nil},
		{"absolute outside sub path", "![a](https://forkana.example/other/attachments/" + uuidA + ")", nil},
		{"relative reference is still accepted", "![a](attachments/" + uuidA + ")", []string{uuidA}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, ExtractAttachmentUUIDs(c.content))
		})
	}
}

func TestExtractAttachmentUUIDsScanCap(t *testing.T) {
	defer test.MockVariableValue(&setting.AppURL, "https://forkana.example/")()
	defer test.MockVariableValue(&setting.AppSubURL, "")()

	ref := "![a](/attachments/" + uuidA + ")"
	within := strings.Repeat("x", MaxScanSize-len(ref)) + ref
	assert.Equal(t, []string{uuidA}, ExtractAttachmentUUIDs(within))

	beyond := strings.Repeat("x", MaxScanSize) + ref
	assert.Empty(t, ExtractAttachmentUUIDs(beyond))
}
