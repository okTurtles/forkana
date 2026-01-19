// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"errors"
	"testing"

	"code.gitea.io/gitea/services/gitdiff"

	"github.com/stretchr/testify/assert"
)

func TestParseOwnerParams(t *testing.T) {
	tests := []struct {
		name        string
		params      string
		wantOwner1  string
		wantOwner2  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid format",
			params:     "alice...bob",
			wantOwner1: "alice",
			wantOwner2: "bob",
			wantErr:    false,
		},
		{
			name:       "valid format with spaces trimmed",
			params:     " alice ... bob ",
			wantOwner1: "alice",
			wantOwner2: "bob",
			wantErr:    false,
		},
		{
			name:        "missing separator",
			params:      "alicebob",
			wantErr:     true,
			errContains: "invalid owner format",
		},
		{
			name:        "wrong separator (two dots)",
			params:      "alice..bob",
			wantErr:     true,
			errContains: "invalid owner format",
		},
		{
			name:        "empty owner1",
			params:      "...bob",
			wantErr:     true,
			errContains: "owner names cannot be empty",
		},
		{
			name:        "empty owner2",
			params:      "alice...",
			wantErr:     true,
			errContains: "owner names cannot be empty",
		},
		{
			name:        "both owners empty",
			params:      "...",
			wantErr:     true,
			errContains: "owner names cannot be empty",
		},
		{
			name:        "empty string",
			params:      "",
			wantErr:     true,
			errContains: "invalid owner format",
		},
		{
			name:       "usernames with hyphens",
			params:     "alice-smith...bob-jones",
			wantOwner1: "alice-smith",
			wantOwner2: "bob-jones",
			wantErr:    false,
		},
		{
			name:       "usernames with underscores",
			params:     "alice_smith...bob_jones",
			wantOwner1: "alice_smith",
			wantOwner2: "bob_jones",
			wantErr:    false,
		},
		{
			name:       "usernames with numbers",
			params:     "alice123...bob456",
			wantOwner1: "alice123",
			wantOwner2: "bob456",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner1, owner2, err := parseOwnerParams(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantOwner1, owner1)
				assert.Equal(t, tt.wantOwner2, owner2)
			}
		})
	}
}

func TestIsReadmeNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "ErrReadmeNotFound",
			err:  ErrReadmeNotFound,
			want: true,
		},
		{
			name: "wrapped ErrReadmeNotFound",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "different error",
			err:  errors.New("different error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isReadmeNotFoundError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildDiffLines(t *testing.T) {
	tests := []struct {
		name           string
		lines1         []string
		lines2         []string
		wantAdditions  int
		wantDeletions  int
		wantPlainLines int
	}{
		{
			name:           "identical content",
			lines1:         []string{"line1", "line2", "line3"},
			lines2:         []string{"line1", "line2", "line3"},
			wantAdditions:  0,
			wantDeletions:  0,
			wantPlainLines: 3,
		},
		{
			name:           "all additions",
			lines1:         []string{},
			lines2:         []string{"line1", "line2"},
			wantAdditions:  2,
			wantDeletions:  0,
			wantPlainLines: 0,
		},
		{
			name:           "all deletions",
			lines1:         []string{"line1", "line2"},
			lines2:         []string{},
			wantAdditions:  0,
			wantDeletions:  2,
			wantPlainLines: 0,
		},
		{
			name:           "mixed changes",
			lines1:         []string{"line1", "line2", "line3"},
			lines2:         []string{"line1", "modified", "line3"},
			wantAdditions:  1,
			wantDeletions:  1,
			wantPlainLines: 2,
		},
		{
			name:           "empty both sides",
			lines1:         []string{},
			lines2:         []string{},
			wantAdditions:  0,
			wantDeletions:  0,
			wantPlainLines: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffLines := buildDiffLines(tt.lines1, tt.lines2)

			// Count line types (skip section header)
			additions := 0
			deletions := 0
			plainLines := 0
			for _, line := range diffLines {
				switch line.Type {
				case gitdiff.DiffLineAdd:
					additions++
				case gitdiff.DiffLineDel:
					deletions++
				case gitdiff.DiffLinePlain:
					plainLines++
				}
			}

			assert.Equal(t, tt.wantAdditions, additions, "additions count mismatch")
			assert.Equal(t, tt.wantDeletions, deletions, "deletions count mismatch")
			assert.Equal(t, tt.wantPlainLines, plainLines, "plain lines count mismatch")
		})
	}
}

func TestPairDiffLinesForSplitView(t *testing.T) {
	tests := []struct {
		name       string
		lines1     []string
		lines2     []string
		wantPairs  int
		checkPairs func(t *testing.T, pairs []SplitViewLine)
	}{
		{
			name:      "identical content shows plain lines on both sides",
			lines1:    []string{"line1", "line2", "line3"},
			lines2:    []string{"line1", "line2", "line3"},
			wantPairs: 3,
			checkPairs: func(t *testing.T, pairs []SplitViewLine) {
				for i, pair := range pairs {
					assert.Equal(t, LineTypePlain, pair.LeftType, "pair %d left should be plain", i)
					assert.Equal(t, LineTypePlain, pair.RightType, "pair %d right should be plain", i)
					assert.Equal(t, pair.LeftContent, pair.RightContent, "pair %d content should match", i)
				}
			},
		},
		{
			name:      "single line modification pairs delete with add",
			lines1:    []string{"line1", "old line", "line3"},
			lines2:    []string{"line1", "new line", "line3"},
			wantPairs: 3,
			checkPairs: func(t *testing.T, pairs []SplitViewLine) {
				// First and last should be plain
				assert.Equal(t, LineTypePlain, pairs[0].LeftType)
				assert.Equal(t, LineTypePlain, pairs[0].RightType)
				assert.Equal(t, LineTypePlain, pairs[2].LeftType)
				assert.Equal(t, LineTypePlain, pairs[2].RightType)
				// Middle should be del/add pair
				assert.Equal(t, LineTypeDel, pairs[1].LeftType)
				assert.Equal(t, LineTypeAdd, pairs[1].RightType)
				assert.Equal(t, "old line", pairs[1].LeftContent)
				assert.Equal(t, "new line", pairs[1].RightContent)
			},
		},
		{
			name:      "multiple consecutive changes pair correctly",
			lines1:    []string{"a", "b", "c"},
			lines2:    []string{"x", "y", "z"},
			wantPairs: 3,
			checkPairs: func(t *testing.T, pairs []SplitViewLine) {
				// All should be del/add pairs
				for i, pair := range pairs {
					assert.Equal(t, LineTypeDel, pair.LeftType, "pair %d left should be del", i)
					assert.Equal(t, LineTypeAdd, pair.RightType, "pair %d right should be add", i)
				}
				assert.Equal(t, "a", pairs[0].LeftContent)
				assert.Equal(t, "x", pairs[0].RightContent)
				assert.Equal(t, "b", pairs[1].LeftContent)
				assert.Equal(t, "y", pairs[1].RightContent)
				assert.Equal(t, "c", pairs[2].LeftContent)
				assert.Equal(t, "z", pairs[2].RightContent)
			},
		},
		{
			name:      "more deletions than additions shows empty on right",
			lines1:    []string{"a", "b", "c"},
			lines2:    []string{"x"},
			wantPairs: 3,
			checkPairs: func(t *testing.T, pairs []SplitViewLine) {
				// First pair: del/add
				assert.Equal(t, LineTypeDel, pairs[0].LeftType)
				assert.Equal(t, LineTypeAdd, pairs[0].RightType)
				// Remaining pairs: del/empty
				assert.Equal(t, LineTypeDel, pairs[1].LeftType)
				assert.Equal(t, LineTypeEmpty, pairs[1].RightType)
				assert.Equal(t, LineTypeDel, pairs[2].LeftType)
				assert.Equal(t, LineTypeEmpty, pairs[2].RightType)
			},
		},
		{
			name:      "more additions than deletions shows empty on left",
			lines1:    []string{"a"},
			lines2:    []string{"x", "y", "z"},
			wantPairs: 3,
			checkPairs: func(t *testing.T, pairs []SplitViewLine) {
				// First pair: del/add
				assert.Equal(t, LineTypeDel, pairs[0].LeftType)
				assert.Equal(t, LineTypeAdd, pairs[0].RightType)
				// Remaining pairs: empty/add
				assert.Equal(t, LineTypeEmpty, pairs[1].LeftType)
				assert.Equal(t, LineTypeAdd, pairs[1].RightType)
				assert.Equal(t, LineTypeEmpty, pairs[2].LeftType)
				assert.Equal(t, LineTypeAdd, pairs[2].RightType)
			},
		},
		{
			name:      "interleaved changes with context",
			lines1:    []string{"header", "old1", "middle", "old2", "footer"},
			lines2:    []string{"header", "new1", "middle", "new2", "footer"},
			wantPairs: 5,
			checkPairs: func(t *testing.T, pairs []SplitViewLine) {
				// header: plain
				assert.Equal(t, LineTypePlain, pairs[0].LeftType)
				assert.Equal(t, "header", pairs[0].LeftContent)
				// old1->new1: del/add
				assert.Equal(t, LineTypeDel, pairs[1].LeftType)
				assert.Equal(t, LineTypeAdd, pairs[1].RightType)
				assert.Equal(t, "old1", pairs[1].LeftContent)
				assert.Equal(t, "new1", pairs[1].RightContent)
				// middle: plain
				assert.Equal(t, LineTypePlain, pairs[2].LeftType)
				assert.Equal(t, "middle", pairs[2].LeftContent)
				// old2->new2: del/add
				assert.Equal(t, LineTypeDel, pairs[3].LeftType)
				assert.Equal(t, LineTypeAdd, pairs[3].RightType)
				assert.Equal(t, "old2", pairs[3].LeftContent)
				assert.Equal(t, "new2", pairs[3].RightContent)
				// footer: plain
				assert.Equal(t, LineTypePlain, pairs[4].LeftType)
				assert.Equal(t, "footer", pairs[4].LeftContent)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diffLines := buildDiffLines(tt.lines1, tt.lines2)
			pairs := pairDiffLinesForSplitView(diffLines)

			assert.Len(t, pairs, tt.wantPairs, "unexpected number of pairs")
			if tt.checkPairs != nil {
				tt.checkPairs(t, pairs)
			}
		})
	}
}
