// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/reqctx"
	"code.gitea.io/gitea/modules/test"
	"code.gitea.io/gitea/services/oauth2_provider"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBasicAuthOAuth2TokenSetsScope ensures that when an OAuth2 token is
// supplied via Basic auth, both IsApiToken and ApiTokenScope are populated,
// preventing a regression of the CVE-2026-28699 scope bypass.
func TestBasicAuthOAuth2TokenSetsScope(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	// Use a symmetric signing key so the generated token can be parsed back
	// by GetOAuthAccessTokenScopeAndUserID through the default signing key.
	signingKey, err := oauth2_provider.CreateJWTSigningKey("HS256", make([]byte, 32))
	require.NoError(t, err)
	defer test.MockVariableValue(&oauth2_provider.DefaultSigningKey, signingKey)()

	grant := unittest.AssertExistsAndLoadBean(t, &auth_model.OAuth2Grant{ID: 1})
	resp, oerr := oauth2_provider.NewAccessTokenResponse(t.Context(), grant, signingKey, signingKey)
	require.Nil(t, oerr)

	store := make(reqctx.ContextData)
	b := &Basic{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user", nil)
	req.SetBasicAuth(resp.AccessToken, "x-oauth-basic")
	u, err := b.Verify(req, httptest.NewRecorder(), store, nil)
	assert.NoError(t, err)
	assert.NotNil(t, u)

	data := store.GetData()
	assert.Equal(t, true, data["IsApiToken"])
	scope, ok := data["ApiTokenScope"].(auth_model.AccessTokenScope)
	assert.True(t, ok, "ApiTokenScope must be set for OAuth2 tokens via Basic auth")
	assert.NotEmpty(t, string(scope))
}
