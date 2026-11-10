// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package private

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternalClientIP(t *testing.T) {
	remoteAddr := func(realIP string) string {
		var got string
		handler := internalClientIP(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			got = req.RemoteAddr
		}))
		req := httptest.NewRequest(http.MethodPost, "/ssh/log", nil)
		req.RemoteAddr = "127.0.0.1:41234"
		if realIP != "" {
			req.Header.Set("X-Real-IP", realIP)
		}
		req.Header.Set("X-Forwarded-For", "203.0.113.9")
		handler.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}

	// the SSH client's address sent by modules/private.NewInternalRequest
	assert.Equal(t, "198.51.100.7", remoteAddr("198.51.100.7"))
	assert.Equal(t, "2001:db8::1", remoteAddr(" 2001:db8::1 "))
	// no or an invalid header keeps the connection's address; X-Forwarded-For is never read
	assert.Equal(t, "127.0.0.1:41234", remoteAddr(""))
	assert.Equal(t, "127.0.0.1:41234", remoteAddr("not-an-ip"))
}
