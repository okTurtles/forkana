// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
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
