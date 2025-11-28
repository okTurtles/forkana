// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
	"unicode/utf8"

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

func TestTruncateToByteLimit(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxBytes  int
		expected  string
		wantValid bool // should result be valid UTF-8
	}{
		{
			name:      "ASCII within limit",
			input:     "hello",
			maxBytes:  10,
			expected:  "hello",
			wantValid: true,
		},
		{
			name:      "ASCII at limit",
			input:     "hello",
			maxBytes:  5,
			expected:  "hello",
			wantValid: true,
		},
		{
			name:      "ASCII over limit",
			input:     "hello world",
			maxBytes:  5,
			expected:  "hello",
			wantValid: true,
		},
		{
			name:      "Japanese text truncated safely",
			input:     "日本語タイトル", // 7 chars, 21 bytes
			maxBytes:  10,
			expected:  "日本語", // 3 chars, 9 bytes (next char would be 12 bytes)
			wantValid: true,
		},
		{
			name:      "Japanese text at exact boundary",
			input:     "日本語", // 3 chars, 9 bytes
			maxBytes:  9,
			expected:  "日本語",
			wantValid: true,
		},
		{
			name:      "Japanese text one byte under",
			input:     "日本語", // 3 chars, 9 bytes
			maxBytes:  8,
			expected:  "日本", // 2 chars, 6 bytes
			wantValid: true,
		},
		{
			name:      "Emoji truncated safely",
			input:     "Hello 🌍🌎🌏", // emoji are 4 bytes each
			maxBytes:  10,
			expected:  "Hello 🌍",
			wantValid: true,
		},
		{
			name:      "Mixed scripts",
			input:     "Hello世界",
			maxBytes:  8,
			expected:  "Hello世", // "Hello" = 5 bytes, "世" = 3 bytes = 8 total, fits exactly
			wantValid: true,
		},
		{
			name:      "Empty string",
			input:     "",
			maxBytes:  10,
			expected:  "",
			wantValid: true,
		},
		{
			name:      "Zero max bytes",
			input:     "hello",
			maxBytes:  0,
			expected:  "",
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateToByteLimit(tt.input, tt.maxBytes)

			if result != tt.expected {
				t.Errorf("truncateToByteLimit(%q, %d) = %q, want %q",
					tt.input, tt.maxBytes, result, tt.expected)
			}

			if len(result) > tt.maxBytes {
				t.Errorf("result length %d exceeds maxBytes %d", len(result), tt.maxBytes)
			}

			if tt.wantValid && !utf8.ValidString(result) {
				t.Errorf("result %q is not valid UTF-8", result)
			}
		})
	}
}

func TestSafeFilename(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		maxLength int
		expected  string
		wantValid bool
	}{
		{
			name:      "simple ASCII",
			title:     "Hello World",
			maxLength: 255,
			expected:  "Hello_World",
			wantValid: true,
		},
		{
			name:      "special characters removed",
			title:     "Hello: World! @#$%",
			maxLength: 255,
			expected:  "Hello_World",
			wantValid: true,
		},
		{
			name:      "Japanese title becomes untitled",
			title:     "日本語タイトル",
			maxLength: 255,
			expected:  "untitled", // \w only matches ASCII, so all chars become _ then trimmed
			wantValid: true,
		},
		{
			name:      "mixed content keeps ASCII",
			title:     "Article about 日本",
			maxLength: 255,
			expected:  "Article_about", // Japanese chars replaced with _, then trimmed
			wantValid: true,
		},
		{
			name:      "empty title",
			title:     "",
			maxLength: 255,
			expected:  "untitled",
			wantValid: true,
		},
		{
			name:      "only special chars",
			title:     "@#$%^&*()",
			maxLength: 255,
			expected:  "untitled",
			wantValid: true,
		},
		{
			name:      "with extension preserved",
			title:     "document.txt",
			maxLength: 255,
			expected:  "document.txt",
			wantValid: true,
		},
		{
			name:      "long name with extension truncated",
			title:     "very_long_document_name.txt",
			maxLength: 15,
			expected:  "very_long_d.txt", // 11 chars base + 4 chars ext = 15
			wantValid: true,
		},
		{
			name:      "emoji in title",
			title:     "Hello 🌍 World",
			maxLength: 255,
			expected:  "Hello_World",
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeFilename(tt.title, tt.maxLength)

			if result != tt.expected {
				t.Errorf("safeFilename(%q, %d) = %q, want %q",
					tt.title, tt.maxLength, result, tt.expected)
			}

			if len(result) > tt.maxLength {
				t.Errorf("result byte length %d exceeds maxLength %d", len(result), tt.maxLength)
			}

			if tt.wantValid && !utf8.ValidString(result) {
				t.Errorf("result %q is not valid UTF-8", result)
			}
		})
	}
}

// TestSafeFilenameUnicodeNeverInvalid ensures Unicode truncation never produces invalid UTF-8
func TestSafeFilenameUnicodeNeverInvalid(t *testing.T) {
	unicodeTitles := []string{
		"日本語タイトル",
		"Ελληνικά",
		"العربية",
		"עברית",
		"中文标题",
		"한국어",
		"🎉🎊🎁🎄🎅",
		"Mixed 日本語 and English",
		"Ümläuts änd spëcîål çhàrs",
	}

	for _, title := range unicodeTitles {
		for maxLen := 1; maxLen <= 30; maxLen++ {
			result := safeFilename(title, maxLen)

			if !utf8.ValidString(result) {
				t.Errorf("safeFilename(%q, %d) = %q is not valid UTF-8",
					title, maxLen, result)
			}

			if len(result) > maxLen && result != "untitled" {
				t.Errorf("safeFilename(%q, %d) = %q exceeds maxLength (len=%d)",
					title, maxLen, result, len(result))
			}
		}
	}
}

func TestProcessResultConstants(t *testing.T) {
	// Verify the process result constants are distinct
	if resultSuccess == resultSkipped {
		t.Error("resultSuccess should not equal resultSkipped")
	}
	if resultSuccess == resultError {
		t.Error("resultSuccess should not equal resultError")
	}
	if resultSkipped == resultError {
		t.Error("resultSkipped should not equal resultError")
	}
}

func TestSkipReasonStrings(t *testing.T) {
	// Verify skip reasons are meaningful strings
	tests := []struct {
		reason   skipReason
		expected string
	}{
		{skipRedirect, "redirect"},
		{skipEmptyContent, "empty_content"},
	}

	for _, tt := range tests {
		if string(tt.reason) != tt.expected {
			t.Errorf("skipReason %v = %q, want %q", tt.reason, string(tt.reason), tt.expected)
		}
	}
}

func TestSkipReasonLoggable(t *testing.T) {
	// Verify skip reasons can be formatted for logging
	reasons := []skipReason{skipRedirect, skipEmptyContent}

	for _, reason := range reasons {
		// Should be non-empty
		if reason == "" {
			t.Error("skip reason should not be empty")
		}

		// Should be safe for tab-separated log format (no tabs or newlines)
		s := string(reason)
		if strings.Contains(s, "\t") {
			t.Errorf("skip reason %q contains tab character", s)
		}
		if strings.Contains(s, "\n") {
			t.Errorf("skip reason %q contains newline character", s)
		}
	}
}

