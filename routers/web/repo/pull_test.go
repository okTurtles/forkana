// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/git"
	files_service "code.gitea.io/gitea/services/repository/files"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmergedRegularFileModesRejectsNonRegularStages(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "symlink", mode: git.EntryModeSymlink.String()},
		{name: "submodule", mode: git.EntryModeCommit.String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := unmergedRegularFileModes([]files_service.UnmergedIndexEntry{
				{Mode: git.EntryModeBlob.String(), Stage: 1, Path: "conflict"},
				{Mode: git.EntryModeBlob.String(), Stage: 2, Path: "conflict"},
				{Mode: tt.mode, Stage: 3, Path: "conflict"},
			})

			var modeErr *unsupportedUnmergedFileModeError
			require.Error(t, err)
			require.ErrorAs(t, err, &modeErr)
			assert.Equal(t, "conflict", modeErr.Path)
			assert.Equal(t, tt.mode, modeErr.Mode)
		})
	}
}

func TestUnmergedRegularFileModesAllowsRegularFiles(t *testing.T) {
	modes, paths, err := unmergedRegularFileModes([]files_service.UnmergedIndexEntry{
		{Mode: git.EntryModeBlob.String(), Stage: 1, Path: "regular.txt"},
		{Mode: git.EntryModeExec.String(), Stage: 2, Path: "regular.txt"},
		{Mode: git.EntryModeBlob.String(), Stage: 3, Path: "regular.txt"},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"regular.txt"}, paths)
	assert.Equal(t, git.EntryModeExec.String(), modes["regular.txt"])
}

func TestComposeMergeCommitMessage(t *testing.T) {
	const defaultTitle = "Article title (#1)"
	const defaultBody = "Reviewed-on: https://forkana.example/user2/repo1/pulls/1\nReviewed-by: user1 <user1@example.com>"

	tests := []struct {
		name      string
		formTitle string
		formBody  string
		expected  string
	}{
		{
			name:     "single click merge keeps the generated title and body",
			expected: defaultTitle + "\n\n" + defaultBody,
		},
		{
			name:      "blank fields are treated as empty",
			formTitle: "  ",
			formBody:  "\n \n",
			expected:  defaultTitle + "\n\n" + defaultBody,
		},
		{
			name:     "a client supplied body replaces the generated one",
			formBody: "custom body",
			expected: defaultTitle + "\n\ncustom body",
		},
		{
			name:      "a client supplied title with no body merges bare",
			formTitle: "custom title",
			expected:  "custom title",
		},
		{
			name:      "a client supplied title and body are used as is",
			formTitle: "custom title",
			formBody:  "custom body",
			expected:  "custom title\n\ncustom body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// the handler only looks the defaults up when the client sends no merge title
			wantDefaultTitle, wantDefaultBody := defaultTitle, defaultBody
			if strings.TrimSpace(tt.formTitle) != "" {
				wantDefaultTitle, wantDefaultBody = "", ""
			}
			assert.Equal(t, tt.expected, composeMergeCommitMessage(tt.formTitle, tt.formBody, wantDefaultTitle, wantDefaultBody))
		})
	}
}
