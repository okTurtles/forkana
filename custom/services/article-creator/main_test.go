// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

func TestExtractYAMLTitle(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "no front matter",
			content:  "# Hello World\n\nSome content",
			expected: "",
		},
		{
			name:     "empty content",
			content:  "",
			expected: "",
		},
		{
			name: "simple unquoted title",
			content: `---
title: Hello World
---

Content here`,
			expected: "Hello World",
		},
		{
			name: "double-quoted title",
			content: `---
title: "Hello World"
---

Content here`,
			expected: "Hello World",
		},
		{
			name: "single-quoted title",
			content: `---
title: 'Hello World'
---

Content here`,
			expected: "Hello World",
		},
		{
			name: "title with escaped double quotes",
			content: `---
title: "Say \"Hello\""
---

Content here`,
			expected: `Say "Hello"`,
		},
		{
			name: "title with escaped single quotes",
			content: `---
title: 'It\'s a test'
---

Content here`,
			expected: "It's a test",
		},
		{
			name: "single double-quote character (edge case - should not panic)",
			content: `---
title: "
---

Content here`,
			expected: `"`,
		},
		{
			name: "single single-quote character (edge case - should not panic)",
			content: `---
title: '
---

Content here`,
			expected: `'`,
		},
		{
			name: "empty quoted string",
			content: `---
title: ""
---

Content here`,
			expected: "",
		},
		{
			name: "empty single-quoted string",
			content: `---
title: ''
---

Content here`,
			expected: "",
		},
		{
			name: "title with spaces",
			content: `---
title:   Spaced Title   
---

Content here`,
			expected: "Spaced Title",
		},
		{
			name: "no title field",
			content: `---
author: John Doe
---

Content here`,
			expected: "",
		},
		{
			name: "incomplete front matter (no closing)",
			content: `---
title: Hello
`,
			expected: "",
		},
		{
			name: "front matter without newline after closing",
			content: `---
title: Hello
---`,
			expected: "Hello",
		},
		{
			name: "title with colons",
			content: `---
title: "Part 1: The Beginning"
---

Content`,
			expected: "Part 1: The Beginning",
		},
		{
			name: "mismatched quotes (starts double, ends single)",
			content: `---
title: "Hello'
---

Content`,
			expected: `"Hello'`,
		},
		{
			name: "mismatched quotes (starts single, ends double)",
			content: `---
title: 'Hello"
---

Content`,
			expected: `'Hello"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractYAMLTitle(tt.content)
			if result != tt.expected {
				t.Errorf("extractYAMLTitle() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestExtractYAMLTitleNoPanic ensures edge cases don't cause panics
func TestExtractYAMLTitleNoPanic(t *testing.T) {
	edgeCases := []string{
		`---
title: "
---`,
		`---
title: '
---`,
		`---
title: 
---`,
		`---
title:
---`,
		"---\ntitle: \"\n---",
		"---\ntitle: '\n---",
	}

	for i, content := range edgeCases {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			// This should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("extractYAMLTitle panicked with input: %q, panic: %v", content, r)
				}
			}()
			_ = extractYAMLTitle(content)
		})
	}
}

