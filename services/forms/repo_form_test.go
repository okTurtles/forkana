// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package forms

import (
	"reflect"
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmitReviewForm_IsEmpty(t *testing.T) {
	cases := []struct {
		form     SubmitReviewForm
		expected bool
	}{
		// Approved PR with a comment shouldn't count as empty
		{SubmitReviewForm{Type: "approve", Content: "Awesome"}, false},

		// Approved PR without a comment shouldn't count as empty
		{SubmitReviewForm{Type: "approve", Content: ""}, false},

		// Rejected PR without a comment should count as empty
		{SubmitReviewForm{Type: "reject", Content: ""}, true},

		// Rejected PR with a comment shouldn't count as empty
		{SubmitReviewForm{Type: "reject", Content: "Awesome"}, false},

		// Comment review on a PR with a comment shouldn't count as empty
		{SubmitReviewForm{Type: "comment", Content: "Awesome"}, false},

		// Comment review on a PR without a comment should count as empty
		{SubmitReviewForm{Type: "comment", Content: ""}, true},
	}

	for _, v := range cases {
		assert.Equal(t, v.expected, v.form.HasEmptyContent())
	}
}

func TestMergePullRequestForm(t *testing.T) {
	expected := &MergePullRequestForm{
		Do:                     "merge",
		MergeTitleField:        "title",
		MergeMessageField:      "message",
		MergeCommitID:          "merge-id",
		HeadCommitID:           "head-id",
		ForceMerge:             true,
		MergeWhenChecksSucceed: true,
		DeleteBranchAfterMerge: true,
	}

	t.Run("NewFields", func(t *testing.T) {
		input := `{
	"do": "merge",
	"merge_title_field": "title",
	"merge_message_field": "message",
	"merge_commit_id": "merge-id",
	"head_commit_id": "head-id",
	"force_merge": true,
	"merge_when_checks_succeed": true,
	"delete_branch_after_merge": true
}`
		var m *MergePullRequestForm
		require.NoError(t, json.Unmarshal([]byte(input), &m))
		assert.Equal(t, expected, m)
	})

	t.Run("OldFields", func(t *testing.T) {
		input := `{
	"Do": "merge",
	"MergeTitleField": "title",
	"MergeMessageField": "message",
	"MergeCommitID": "merge-id",
	"head_commit_id": "head-id",
	"force_merge": true,
	"merge_when_checks_succeed": true,
	"delete_branch_after_merge": true
}`
		var m *MergePullRequestForm
		require.NoError(t, json.Unmarshal([]byte(input), &m))
		assert.Equal(t, expected, m)
	})
}

// TestMergePullRequestFormUnmarshalCoversAllFields fails when a field is added to
// MergePullRequestForm but not to the aux struct in its UnmarshalJSON.
func TestMergePullRequestFormUnmarshalCoversAllFields(t *testing.T) {
	formType := reflect.TypeFor[MergePullRequestForm]()
	body := map[string]any{}
	for i := range formType.NumField() {
		field := formType.Field(i)
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		require.NotEmpty(t, key, "field %s has no json tag", field.Name)
		switch field.Type.Kind() {
		case reflect.Bool:
			body[key] = true
		case reflect.String:
			body[key] = "x"
		default:
			t.Fatalf("field %s has unhandled kind %s; extend this test", field.Name, field.Type.Kind())
		}
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	var f MergePullRequestForm
	require.NoError(t, json.Unmarshal(raw, &f))
	v := reflect.ValueOf(f)
	for i := range formType.NumField() {
		assert.False(t, v.Field(i).IsZero(), "field %s is not populated by UnmarshalJSON", formType.Field(i).Name)
	}
}
