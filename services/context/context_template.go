// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"context"
	"time"
)

var _ context.Context = TemplateContext(nil)

func NewTemplateContext(ctx context.Context) TemplateContext {
	return TemplateContext{"_ctx": ctx}
}

func (c TemplateContext) parentContext() context.Context {
	if parent, ok := c["_ctx"].(context.Context); ok {
		return parent
	}
	// The error pages (500, 503) and the install page render with a minimal template
	// context that has no request context. A template asking it for a value (an
	// absolute url in base/head_opengraph) then gets none instead of a panic.
	return context.Background()
}

func (c TemplateContext) Deadline() (deadline time.Time, ok bool) {
	return c.parentContext().Deadline()
}

func (c TemplateContext) Done() <-chan struct{} {
	return c.parentContext().Done()
}

func (c TemplateContext) Err() error {
	return c.parentContext().Err()
}

func (c TemplateContext) Value(key any) any {
	return c.parentContext().Value(key)
}
