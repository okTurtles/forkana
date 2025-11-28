// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEscapeYAMLString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "double quotes",
			input:    `Say "Hello"`,
			expected: `Say \"Hello\"`,
		},
		{
			name:     "backslash",
			input:    `C:\path\to\file`,
			expected: `C:\\path\\to\\file`,
		},
		{
			name:     "backslash before quote",
			input:    `path\"quoted`,
			expected: `path\\\"quoted`,
		},
		{
			name:     "newline",
			input:    "line1\nline2",
			expected: `line1\nline2`,
		},
		{
			name:     "carriage return",
			input:    "line1\rline2",
			expected: `line1\rline2`,
		},
		{
			name:     "tab",
			input:    "col1\tcol2",
			expected: `col1\tcol2`,
		},
		{
			name:     "mixed special characters",
			input:    "Title with \"quotes\", backslash\\, and\nnewline",
			expected: `Title with \"quotes\", backslash\\, and\nnewline`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unicode characters",
			input:    "日本語タイトル",
			expected: "日本語タイトル",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeYAMLString(tt.input)
			if result != tt.expected {
				t.Errorf("escapeYAMLString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAddFrontMatter(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		body        string
		checkFields map[string]string // fields to verify in parsed YAML
	}{
		{
			name:  "simple title",
			title: "Hello World",
			body:  "# Content",
			checkFields: map[string]string{
				"title":   "Hello World",
				"license": "CC BY-SA 4.0",
			},
		},
		{
			name:  "title with quotes",
			title: `Say "Hello"`,
			body:  "# Content",
			checkFields: map[string]string{
				"title": `Say "Hello"`,
			},
		},
		{
			name:  "title with backslash",
			title: `C:\path`,
			body:  "# Content",
			checkFields: map[string]string{
				"title": `C:\path`,
			},
		},
		{
			name:  "title with special characters",
			title: "Title: A \"Test\" with\nnewline",
			body:  "# Content",
			checkFields: map[string]string{
				"title": "Title: A \"Test\" with\nnewline",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addFrontMatter(tt.title, tt.body)

			// Verify front matter structure
			if !strings.HasPrefix(result, "---\n") {
				t.Error("front matter should start with ---")
			}

			// Extract front matter
			parts := strings.SplitN(result, "---", 3)
			if len(parts) < 3 {
				t.Fatal("invalid front matter structure")
			}

			frontMatter := strings.TrimSpace(parts[1])

			// Parse YAML to verify it's valid
			var parsed map[string]any
			if err := yaml.Unmarshal([]byte(frontMatter), &parsed); err != nil {
				t.Fatalf("failed to parse YAML front matter: %v\nfront matter:\n%s", err, frontMatter)
			}

			// Check expected fields
			for field, expected := range tt.checkFields {
				if val, ok := parsed[field]; !ok {
					t.Errorf("missing field %q in front matter", field)
				} else if val != expected {
					t.Errorf("field %q = %q, want %q", field, val, expected)
				}
			}

			// Verify body is appended
			if !strings.HasSuffix(result, tt.body) {
				t.Error("body should be appended after front matter")
			}
		})
	}
}

func TestAddFrontMatterSourceURL(t *testing.T) {
	// Test that source URL is properly escaped
	title := "Article with spaces & special chars"
	result := addFrontMatter(title, "body")

	// Extract and parse front matter
	parts := strings.SplitN(result, "---", 3)
	if len(parts) < 3 {
		t.Fatal("invalid front matter structure")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(parts[1]), &parsed); err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}

	source, ok := parsed["source"].(string)
	if !ok {
		t.Fatal("source field not found or not a string")
	}

	// Verify URL encoding
	if strings.Contains(source, " ") {
		t.Error("source URL should not contain unencoded spaces")
	}
	if !strings.Contains(source, "Article_with_spaces") {
		t.Error("source URL should contain underscored title")
	}
}

