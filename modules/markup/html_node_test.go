// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessNodeAttrID_HTMLHeadingWithoutID(t *testing.T) {
	// HTML headings without an id get one generated from their text content
	// See: https://github.com/go-gitea/gitea/issues/27383
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "h1 without id",
			input:    `<h1>Heading without ID</h1>`,
			expected: `<h1 id="user-content-heading-without-id">Heading without ID</h1>`,
		},
		{
			name:     "h2 without id",
			input:    `<h2>Another Heading</h2>`,
			expected: `<h2 id="user-content-another-heading">Another Heading</h2>`,
		},
		{
			name:     "h3 with nested elements",
			input:    `<h3>Heading with <em>emphasis</em> and <a href="https://example.com">link</a></h3>`,
			expected: `<h3 id="user-content-heading-with-emphasis-and-link">Heading with <em>emphasis</em> and <a href="https://example.com">link</a></h3>`,
		},
		{
			name:     "h6 with punctuation and unicode",
			input:    `<h6>Ação &amp; reação!</h6>`,
			expected: `<h6 id="user-content-ação--reação">Ação &amp; reação!</h6>`,
		},
		{
			name:     "heading with existing id keeps it",
			input:    `<h1 id="my-custom-id">Custom ID Heading</h1>`,
			expected: `<h1 id="user-content-my-custom-id">Custom ID Heading</h1>`,
		},
		{
			name:     "heading with existing user-content id is unchanged",
			input:    `<h2 id="user-content-already">Already</h2>`,
			expected: `<h2 id="user-content-already">Already</h2>`,
		},
		{
			name:     "empty heading gets no id",
			input:    `<h1></h1>`,
			expected: `<h1></h1>`,
		},
		{
			name:     "heading without id-able text gets no id",
			input:    `<h1>!!!</h1>`,
			expected: `<h1>!!!</h1>`,
		},
		{
			name:     "footnotes heading gets no id",
			input:    `<h2 class="footnotes-heading">References</h2>`,
			expected: `<h2 class="footnotes-heading">References</h2>`,
		},
		{
			name:     "duplicate headings are not deduplicated",
			input:    `<h2>Notes</h2><h2>Notes</h2>`,
			expected: `<h2 id="user-content-notes">Notes</h2><h2 id="user-content-notes">Notes</h2>`,
		},
		{
			name:     "non-heading elements get no id",
			input:    `<p>Paragraph</p><div>Div</div>`,
			expected: `<p>Paragraph</p><div>Div</div>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result strings.Builder
			ctx := NewTestRenderContext().WithEnableHeadingIDGeneration(true)
			err := PostProcessDefault(ctx, strings.NewReader(tc.input), &result)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, strings.TrimSpace(result.String()))
		})
	}
}

func TestProcessNodeAttrID_SkipHeadingIDGenerationWhenDisabled(t *testing.T) {
	var result strings.Builder
	ctx := NewTestRenderContext()
	err := PostProcessDefault(ctx, strings.NewReader(`<h1>Heading without ID</h1>`), &result)
	require.NoError(t, err)
	assert.Equal(t, `<h1>Heading without ID</h1>`, strings.TrimSpace(result.String()))
}
