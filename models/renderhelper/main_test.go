// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package renderhelper

import (
	"context"
	"strings"
	"testing"

	"code.gitea.io/gitea/models/unittest"
	"code.gitea.io/gitea/modules/markup"

	_ "code.gitea.io/gitea/models/repo" // register repo models including Subject
)

func TestMain(m *testing.M) {
	unittest.MainTest(m, &unittest.TestOptions{
		SetUp: func() error {
			markup.RenderBehaviorForTesting.DisableAdditionalAttributes = true
			markup.Init(&markup.RenderHelperFuncs{
				IsUsernameMentionable: func(ctx context.Context, username string) bool {
					return username == "user2"
				},
			})
			return nil
		},
	})
}

func testRenderString(ctx *markup.RenderContext, content string) (string, error) {
	var buf strings.Builder
	if err := markup.Render(ctx, strings.NewReader(content), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
