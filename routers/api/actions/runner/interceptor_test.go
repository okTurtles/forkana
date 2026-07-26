// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package runner

import "testing"

func TestGetMethodNameFromProcedure(t *testing.T) {
	tests := []struct {
		name      string
		procedure string
		want      string
	}{
		{
			name:      "valid runner procedure",
			procedure: "/runner.v1.RunnerService/Register",
			want:      "Register",
		},
		{
			// connect's Spec().Procedure never contains a URL prefix in production,
			// but a multi-segment path still resolves to its last segment
			name:      "multi-segment path resolves last segment",
			procedure: "/api/actions/runner.v1.RunnerService/UpdateTask",
			want:      "UpdateTask",
		},
		{
			name:      "missing leading slash",
			procedure: "runner.v1.RunnerService/Register",
			want:      "",
		},
		{
			name:      "empty service segment",
			procedure: "//Register",
			want:      "",
		},
		{
			name:      "method only",
			procedure: "Register",
			want:      "",
		},
		{
			name:      "missing service",
			procedure: "/Register",
			want:      "",
		},
		{
			name:      "missing method",
			procedure: "/runner.v1.RunnerService/",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getMethodNameFromProcedure(tt.procedure); got != tt.want {
				t.Fatalf("getMethodNameFromProcedure(%q) = %q, want %q", tt.procedure, got, tt.want)
			}
		})
	}
}
