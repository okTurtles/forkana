// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package forms

import (
	"strings"
	"testing"

	"code.gitea.io/gitea/modules/optional"

	"github.com/stretchr/testify/assert"
)

// #475: the article editors submit a required summary of up to 200 characters
func TestEditRepoFileForm_ArticleSummary(t *testing.T) {
	for _, c := range []struct {
		name     string
		summary  optional.Option[string]
		required bool
		want     string
		wantOK   bool
		wantErr  error
	}{
		{"not an article editor", optional.None[string](), false, "", false, nil},
		{"article editor without the field", optional.None[string](), true, "", true, ErrArticleSummaryRequired},
		{"empty", optional.Some(""), false, "", true, ErrArticleSummaryRequired},
		{"blanks only", optional.Some(" \t\r\n "), true, "", true, ErrArticleSummaryRequired},
		{"trimmed, one line", optional.Some("  The Moon,\r\n Earth's   satellite. "), true, "The Moon, Earth's satellite.", true, nil},
		{"200 characters", optional.Some(strings.Repeat("é", 200)), true, strings.Repeat("é", 200), true, nil},
		{"201 characters", optional.Some(strings.Repeat("é", 201)), true, "", true, ErrArticleSummaryTooLong},
	} {
		t.Run(c.name, func(t *testing.T) {
			form := &EditRepoFileForm{Summary: c.summary}
			got, ok, err := form.ArticleSummary(c.required)
			assert.Equal(t, c.want, got)
			assert.Equal(t, c.wantOK, ok)
			assert.ErrorIs(t, err, c.wantErr)
		})
	}

	// a stored description is compared in the same form
	assert.Equal(t, "Line one line two", NormalizeArticleSummary("  Line one\r\n\tline  two "))
	assert.Empty(t, NormalizeArticleSummary(" \n "))
}
