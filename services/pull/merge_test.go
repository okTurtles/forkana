// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_expandDefaultMergeMessage(t *testing.T) {
	type args struct {
		template string
		vars     map[string]string
	}
	tests := []struct {
		name     string
		args     args
		want     string
		wantBody string
	}{
		{
			name: "single line",
			args: args{
				template: "Merge ${PullRequestTitle}",
				vars: map[string]string{
					"PullRequestTitle":       "PullRequestTitle",
					"PullRequestDescription": "Pull\nRequest\nDescription\n",
				},
			},
			want:     "Merge PullRequestTitle",
			wantBody: "",
		},
		{
			name: "multiple lines",
			args: args{
				template: "Merge ${PullRequestTitle}\nDescription:\n\n${PullRequestDescription}\n",
				vars: map[string]string{
					"PullRequestTitle":       "PullRequestTitle",
					"PullRequestDescription": "Pull\nRequest\nDescription\n",
				},
			},
			want:     "Merge PullRequestTitle",
			wantBody: "Description:\n\nPull\nRequest\nDescription\n",
		},
		{
			name: "leading newlines",
			args: args{
				template: "\n\n\nMerge ${PullRequestTitle}\n\n\nDescription:\n\n${PullRequestDescription}\n",
				vars: map[string]string{
					"PullRequestTitle":       "PullRequestTitle",
					"PullRequestDescription": "Pull\nRequest\nDescription\n",
				},
			},
			want:     "Merge PullRequestTitle",
			wantBody: "Description:\n\nPull\nRequest\nDescription\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := expandDefaultMergeMessage(tt.args.template, tt.args.vars)
			assert.Equalf(t, tt.want, got, "expandDefaultMergeMessage(%v, %v)", tt.args.template, tt.args.vars)
			assert.Equalf(t, tt.wantBody, got1, "expandDefaultMergeMessage(%v, %v)", tt.args.template, tt.args.vars)
		})
	}
}

func TestAddCommitMessageTailer(t *testing.T) {
	// add tailer for empty message
	assert.Equal(t, "\n\nTest-tailer: TestValue", AddCommitMessageTailer("", "Test-tailer", "TestValue"))

	// add tailer for message without newlines
	assert.Equal(t, "title\n\nTest-tailer: TestValue", AddCommitMessageTailer("title", "Test-tailer", "TestValue"))
	assert.Equal(t, "title\n\nNot tailer: xxx\n\nTest-tailer: TestValue", AddCommitMessageTailer("title\n\nNot tailer: xxx", "Test-tailer", "TestValue"))
	assert.Equal(t, "title\n\nNotTailer: xxx\n\nTest-tailer: TestValue", AddCommitMessageTailer("title\n\nNotTailer: xxx", "Test-tailer", "TestValue"))
	assert.Equal(t, "title\n\nnot-tailer: xxx\n\nTest-tailer: TestValue", AddCommitMessageTailer("title\n\nnot-tailer: xxx", "Test-tailer", "TestValue"))

	// add tailer for message with one EOL
	assert.Equal(t, "title\n\nTest-tailer: TestValue", AddCommitMessageTailer("title\n", "Test-tailer", "TestValue"))

	// add tailer for message with two EOLs
	assert.Equal(t, "title\n\nTest-tailer: TestValue", AddCommitMessageTailer("title\n\n", "Test-tailer", "TestValue"))

	// add tailer for message with existing tailer (won't duplicate)
	assert.Equal(t, "title\n\nTest-tailer: TestValue", AddCommitMessageTailer("title\n\nTest-tailer: TestValue", "Test-tailer", "TestValue"))
	assert.Equal(t, "title\n\nTest-tailer: TestValue\n", AddCommitMessageTailer("title\n\nTest-tailer: TestValue\n", "Test-tailer", "TestValue"))

	// add tailer for message with existing tailer and different value (will append)
	assert.Equal(t, "title\n\nTest-tailer: v1\nTest-tailer: v2", AddCommitMessageTailer("title\n\nTest-tailer: v1", "Test-tailer", "v2"))
	assert.Equal(t, "title\n\nTest-tailer: v1\nTest-tailer: v2", AddCommitMessageTailer("title\n\nTest-tailer: v1\n", "Test-tailer", "v2"))
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
			assert.Equal(t, tt.expected, ComposeMergeCommitMessage(tt.formTitle, tt.formBody, wantDefaultTitle, wantDefaultBody))
		})
	}
}
