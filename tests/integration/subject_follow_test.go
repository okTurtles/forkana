// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	activities_model "code.gitea.io/gitea/models/activities"
	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unittest"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubjectFollowToggle (#165): the subject header's Follow follows the subject, on its
// own: it does not watch the article shown, and the Article view's Watch does not follow
// the subject. Both answer with themselves only.
func TestSubjectFollowToggle(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	_, repo, subjectName := loadArticleRepo(t, 1)
	require.NotZero(t, repo.SubjectID)
	user5 := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user5"})
	require.False(t, repo_model.IsWatching(t.Context(), user5.ID, repo.ID))
	session := loginUser(t, user5.Name)

	isFollowing := func(t *testing.T) bool {
		following, err := repo_model.IsFollowingSubject(t.Context(), user5.ID, repo.SubjectID)
		require.NoError(t, err)
		return following
	}

	followForm := func(t *testing.T) (action, label string) {
		_, body := getSubjectPage(t, session, subjectName, "view=article")
		doc := NewHTMLParser(t, bytes.NewBufferString(body))
		form := doc.Find(".repo-header-follow form")
		require.Equal(t, 1, form.Length())
		return form.AttrOr("action", ""), form.Find(".subject-follow-button").AttrOr("aria-label", "")
	}

	followLink := fmt.Sprintf("/-/subject/%d", repo.SubjectID)
	action, label := followForm(t)
	assert.Equal(t, followLink+"/follow", action)
	assert.Equal(t, "Follow", label)

	req := NewRequestWithValues(t, "POST", action, map[string]string{"_csrf": GetUserCSRFToken(t, session)})
	answer := NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
	// the answer is the Follow button, now following, and nothing else
	assert.Equal(t, followLink+"/unfollow", answer.Find("form").AttrOr("action", ""))
	assert.Equal(t, "Following", answer.Find(".subject-follow-button").AttrOr("aria-label", ""))
	assert.Equal(t, 1, answer.Find("form").Length())
	assert.Equal(t, 0, answer.Find(`[hx-swap-oob]`).Length())
	assert.True(t, isFollowing(t))
	assert.False(t, repo_model.IsWatching(t.Context(), user5.ID, repo.ID), "following the subject does not watch the article")

	action, label = followForm(t)
	assert.Equal(t, followLink+"/unfollow", action)
	assert.Equal(t, "Following", label)

	// the article's Watch is still "Watch", and pressing it does not change the follow
	_, body := getSubjectPage(t, session, subjectName, "view=article")
	doc := NewHTMLParser(t, bytes.NewBufferString(body))
	watch := doc.Find(`#article-tabs form.article-watch-form`)
	require.Equal(t, 1, watch.Length())
	assert.Equal(t, "Watch", strings.TrimSpace(watch.Find(`[data-role="article-watch"] span`).Text()))
	req = NewRequestWithValues(t, "POST", watch.AttrOr("action", ""), map[string]string{"_csrf": GetUserCSRFToken(t, session)})
	session.MakeRequest(t, req, http.StatusOK)
	assert.True(t, repo_model.IsWatching(t.Context(), user5.ID, repo.ID))
	assert.True(t, isFollowing(t))

	req = NewRequestWithValues(t, "POST", followLink+"/unfollow", map[string]string{"_csrf": GetUserCSRFToken(t, session)})
	answer = NewHTMLParser(t, session.MakeRequest(t, req, http.StatusOK).Body)
	assert.Equal(t, "Follow", answer.Find(".subject-follow-button").AttrOr("aria-label", ""))
	assert.False(t, isFollowing(t))
	assert.True(t, repo_model.IsWatching(t.Context(), user5.ID, repo.ID), "unfollowing the subject does not unwatch the article")

	// a subject that does not exist, no subject (xorm would take any subject for id 0), and
	// a subject without an article the reader can read (subject 2 has none) are not found
	for _, id := range []string{"999999", "0", "2"} {
		req = NewRequestWithValues(t, "POST", "/-/subject/"+id+"/follow", map[string]string{"_csrf": GetUserCSRFToken(t, session)})
		session.MakeRequest(t, req, http.StatusNotFound)
	}
	assert.Equal(t, 0, unittest.GetCount(t, &repo_model.SubjectFollow{UserID: user5.ID}))

	// a signed-out reader (with the page's CSRF token) is sent to sign in, and follows
	// nothing; without the token, the request is refused before that
	guest := emptyTestSession(t)
	req = NewRequestWithValues(t, "POST", followLink+"/follow", map[string]string{"_csrf": GetAnonymousCSRFToken(t, guest)})
	resp := guest.MakeRequest(t, req, http.StatusSeeOther)
	assert.Contains(t, resp.Header().Get("Location"), "/user/login")
	MakeRequest(t, NewRequest(t, "POST", followLink+"/follow"), http.StatusBadRequest)
	assert.Equal(t, 0, unittest.GetCount(t, &repo_model.SubjectFollow{SubjectID: repo.SubjectID}))
}

// TestSubjectFollowFeed (#165): the activity on an article of a followed subject is in the
// follower's feed (the /feeds page and the home page), though they do not watch it.
func TestSubjectFollowFeed(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	require.NotZero(t, repo.SubjectID)
	const sha = "65f1bf27bc3bf70f64657658635e66094edbcb4d"
	content, err := json.Marshal(&repository.PushCommits{
		Commits: []*repository.PushCommit{{Sha1: sha, Message: "followed subject commit"}},
		Len:     1,
	})
	require.NoError(t, err)
	// user4's push: only user4's own copy, user5 does not watch the article
	require.NoError(t, db.Insert(t.Context(), &activities_model.Action{
		UserID:    4,
		ActUserID: 4,
		OpType:    activities_model.ActionCommitRepo,
		RepoID:    repo.ID,
		RefName:   "refs/heads/master",
		Content:   string(content),
	}))

	session := loginUser(t, "user5")
	// the /feeds page links the push's commit; the home page's three latest items say who
	// did what
	feedsHas := func(t *testing.T) bool {
		resp := session.MakeRequest(t, NewRequest(t, "GET", "/feeds"), http.StatusOK)
		return strings.Contains(resp.Body.String(), "?version="+sha)
	}
	homeHas := func(t *testing.T) bool {
		resp := session.MakeRequest(t, NewRequest(t, "GET", "/"), http.StatusOK)
		return NewHTMLParser(t, resp.Body).Find(`.home-feed-item .home-feed-item-action a[href="/user4"]`).Length() > 0
	}
	assert.False(t, feedsHas(t))
	assert.False(t, homeHas(t))

	req := NewRequestWithValues(t, "POST", fmt.Sprintf("/-/subject/%d/follow", repo.SubjectID), map[string]string{"_csrf": GetUserCSRFToken(t, session)})
	session.MakeRequest(t, req, http.StatusOK)
	assert.True(t, feedsHas(t))
	assert.True(t, homeHas(t))
}
