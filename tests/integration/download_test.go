// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/tests"

	"github.com/stretchr/testify/assert"
)

func TestDownloadByID(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")

	// Request raw blob
	req := NewRequest(t, "GET", "/user2/repo1/raw/blob/4b4851ad51df6a7d9f25c979345979eaeb5b349f")
	resp := session.MakeRequest(t, req, http.StatusOK)

	assert.Equal(t, "# repo1\n\nDescription for repo1", resp.Body.String())
}

func TestDownloadByIDForSVGUsesSecureHeaders(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")

	// Request raw blob
	req := NewRequest(t, "GET", "/user2/repo2/raw/blob/6395b68e1feebb1e4c657b4f9f6ba2676a283c0b")
	resp := session.MakeRequest(t, req, http.StatusOK)

	assert.Equal(t, "default-src 'none'; style-src 'unsafe-inline'; sandbox", resp.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "image/svg+xml", resp.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", resp.Header().Get("X-Content-Type-Options"))
}

func TestDownloadByIDMedia(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")

	// Request raw blob
	req := NewRequest(t, "GET", "/user2/repo1/media/blob/4b4851ad51df6a7d9f25c979345979eaeb5b349f")
	resp := session.MakeRequest(t, req, http.StatusOK)

	assert.Equal(t, "# repo1\n\nDescription for repo1", resp.Body.String())
}

func TestDownloadByIDMediaForSVGUsesSecureHeaders(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")

	// Request raw blob
	req := NewRequest(t, "GET", "/user2/repo2/media/blob/6395b68e1feebb1e4c657b4f9f6ba2676a283c0b")
	resp := session.MakeRequest(t, req, http.StatusOK)

	assert.Equal(t, "default-src 'none'; style-src 'unsafe-inline'; sandbox", resp.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "image/svg+xml", resp.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", resp.Header().Get("X-Content-Type-Options"))
}

func TestDownloadRawTextFileWithoutMimeTypeMapping(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")

	req := NewRequest(t, "GET", "/user2/repo2/raw/branch/master/test.xml")
	resp := session.MakeRequest(t, req, http.StatusOK)

	assert.Equal(t, "text/plain; charset=utf-8", resp.Header().Get("Content-Type"))
}

func TestDownloadRawTextFileWithMimeTypeMapping(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	setting.MimeTypeMap.Map[".xml"] = "text/xml"
	setting.MimeTypeMap.Enabled = true

	session := loginUser(t, "user2")

	req := NewRequest(t, "GET", "/user2/repo2/raw/branch/master/test.xml")
	resp := session.MakeRequest(t, req, http.StatusOK)

	assert.Equal(t, "text/xml; charset=utf-8", resp.Header().Get("Content-Type"))

	delete(setting.MimeTypeMap.Map, ".xml")
	setting.MimeTypeMap.Enabled = false
}

// newTokenBasicAuthRequest sends an API token as the basic auth username, the way git clients do.
func newTokenBasicAuthRequest(t testing.TB, method, urlStr, token string) *RequestWrapper {
	req := NewRequest(t, method, urlStr)
	req.SetBasicAuth(token, "x-oauth-basic")
	return req
}

type downloadScopeCase struct {
	name         string
	method       string
	url          string
	publicOnlyOK bool
}

func TestDownloadRepoContentTokenScopes(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	ownerReadToken := getUserToken(t, "user2", auth_model.AccessTokenScopeReadRepository)
	miscToken := getUserToken(t, "user2", auth_model.AccessTokenScopeReadMisc)
	publicOnlyToken := getUserToken(t, "user2", auth_model.AccessTokenScopeReadRepository, auth_model.AccessTokenScopePublicOnly)

	cases := []downloadScopeCase{
		{"PublicArchiveDownload", http.MethodGet, "/user2/repo1/archive/master.tar.gz", true},
		{"PrivateArchiveDownload", http.MethodGet, "/user2/repo2/archive/master.tar.gz", false},
		{"PublicRawBlob", http.MethodGet, "/user2/repo1/raw/blob/4b4851ad51df6a7d9f25c979345979eaeb5b349f", true},
		{"PublicRawBranch", http.MethodGet, "/user2/repo1/raw/branch/master/README.md", true},
		{"PublicRawTag", http.MethodGet, "/user2/repo1/raw/tag/v1.1/README.md", true},
		{"PublicRawCommit", http.MethodGet, "/user2/repo1/raw/commit/65f1bf27bc3bf70f64657658635e66094edbcb4d/README.md", true},
		{"PrivateRawBranch", http.MethodGet, "/user2/repo2/raw/branch/master/test.xml", false},
		{"PrivateRawBlob", http.MethodGet, "/user2/repo2/raw/blob/6395b68e1feebb1e4c657b4f9f6ba2676a283c0b", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			MakeRequest(t, NewRequest(t, tc.method, tc.url).AddTokenAuth(miscToken), http.StatusForbidden)
			MakeRequest(t, newTokenBasicAuthRequest(t, tc.method, tc.url, miscToken), http.StatusForbidden)
			MakeRequest(t, NewRequest(t, tc.method, tc.url).AddTokenAuth(ownerReadToken), http.StatusOK)

			publicOnlyStatus := http.StatusForbidden
			if tc.publicOnlyOK {
				publicOnlyStatus = http.StatusOK
			}
			MakeRequest(t, NewRequest(t, tc.method, tc.url).AddTokenAuth(publicOnlyToken), publicOnlyStatus)
		})
	}

	// Unlike upstream, Forkana's web CSRF check rejects token-authenticated POSTs before they reach
	// InitiateDownload, so its scope check is defense in depth and a token cannot start an archive.
	t.Run("ArchiveInitiateRejectsTokens", func(t *testing.T) {
		MakeRequest(t, NewRequest(t, http.MethodPost, "/user2/repo2/archive/master.tar.gz").AddTokenAuth(miscToken), http.StatusBadRequest)
		MakeRequest(t, NewRequest(t, http.MethodPost, "/user2/repo2/archive/master.tar.gz").AddTokenAuth(ownerReadToken), http.StatusBadRequest)
	})

	// "/media/" is not a token-authenticated path (see services/auth/auth.go), so the token is
	// ignored and the request is served as anonymous: public content stays readable, private does not.
	t.Run("MediaIgnoresToken", func(t *testing.T) {
		MakeRequest(t, NewRequest(t, "GET", "/user2/repo1/media/branch/master/README.md").AddTokenAuth(miscToken), http.StatusOK)
		MakeRequest(t, NewRequest(t, "GET", "/user2/repo2/media/branch/master/test.xml").AddTokenAuth(ownerReadToken), http.StatusNotFound)
	})
}
