// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/tests"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

// TestHelpWhatCanBeReported (#541): the page is public and lists every reason of the report
// form but "Something else", in the form's order, under the reasons' values as anchors.
func TestHelpWhatCanBeReported(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	resp := MakeRequest(t, NewRequest(t, "GET", "/help/what-can-be-reported"), http.StatusOK)
	doc := NewHTMLParser(t, resp.Body)

	var ids []string
	doc.Find("section.help-reason").Each(func(_ int, s *goquery.Selection) {
		ids = append(ids, s.AttrOr("id", ""))
	})
	assert.Equal(t, []string{
		"child-safety", "hate-or-harassment", "doxxing", "graphic-violence", "adult-sexual-content",
		"illegal-in-region", "no-reliable-source", "plagiarism-or-copyright", "off-topic-or-spam",
	}, ids)
	assert.Equal(t, 9, doc.Find(".help-jump a.help-reason-label").Length())
	assert.Equal(t, "/help/what-can-be-reported", doc.Find(".help-nav-item.active").AttrOr("href", ""))
	assert.Equal(t, 1, doc.Find("#graphic-violence .help-cw").Length())

	// "help" is reserved for these pages
	assert.Error(t, user_model.IsUsableUsername("help"))
}

// TestHelpBackLink (#541): Back returns to the page given in redirect_to, only if it is a
// page of this site; otherwise it leads home.
func TestHelpBackLink(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	back := func(query string) string {
		resp := MakeRequest(t, NewRequest(t, "GET", "/help/what-can-be-reported"+query), http.StatusOK)
		return NewHTMLParser(t, resp.Body).Find("a.help-back").AttrOr("href", "")
	}
	assert.Equal(t, "/", back(""))
	assert.Equal(t, "/subject/example-subject/user2", back("?redirect_to=%2Fsubject%2Fexample-subject%2Fuser2"))
	assert.Equal(t, "/", back("?redirect_to=https%3A%2F%2Fexample.com%2F"))
	assert.Equal(t, "/", back("?redirect_to=%2F%2Fexample.com%2F"))
}
