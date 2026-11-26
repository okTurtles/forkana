// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package moderation

import (
	"strings"
	"testing"
	"unicode/utf8"

	moderation_model "code.gitea.io/gitea/models/moderation"
	repo_model "code.gitea.io/gitea/models/repo"
	user_model "code.gitea.io/gitea/models/user"

	"github.com/stretchr/testify/assert"
)

// The section and title columns are VARCHAR(255): a long heading must not make the insert fail
// on databases that enforce it.
func TestSectionLengths(t *testing.T) {
	long := strings.Repeat("Very long heading ", 30) // 540 characters

	title := headingTitle(t.Context(), []byte("# "+long+"\n\nText.\n"), "user-content-"+strings.TrimSuffix(strings.ReplaceAll(strings.ToLower(long), " ", "-"), "-"))
	assert.NotEmpty(t, title)
	assert.LessOrEqual(t, utf8.RuneCountInString(title), moderation_model.MaxSectionLength)

	repo := &repo_model.Repository{Name: "article", Owner: &user_model.User{Name: "owner"}}
	caseName := caseTitle(repo, "user-content-x", long, moderation_model.ReasonDoxxing)
	assert.LessOrEqual(t, utf8.RuneCountInString(caseName), 255)

	assert.NoError(t, checkSectionAnchor("user-content-short"))
	assert.Error(t, checkSectionAnchor(strings.Repeat("a", moderation_model.MaxSectionLength+1)))
}
