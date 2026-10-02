// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cache

import (
	"context"
	"testing"
	"time"

	"code.gitea.io/gitea/modules/test"

	"github.com/stretchr/testify/assert"
)

func TestWithCacheContext(t *testing.T) {
	ctx := WithCacheContext(t.Context())
	c := GetContextCache(ctx)
	v, _ := c.Get("empty_field", "my_config1")
	assert.Nil(t, v)

	const field = "system_setting"
	v, _ = c.Get(field, "my_config1")
	assert.Nil(t, v)
	c.Put(field, "my_config1", 1)
	v, _ = c.Get(field, "my_config1")
	assert.NotNil(t, v)
	assert.Equal(t, 1, v.(int))

	c.Delete(field, "my_config1")
	c.Delete(field, "my_config2") // remove a non-exist key

	v, _ = c.Get(field, "my_config1")
	assert.Nil(t, v)

	vInt, err := GetWithContextCache(ctx, field, "my_config1", func(context.Context, string) (int, error) {
		return 1, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, vInt)

	v, _ = c.Get(field, "my_config1")
	assert.EqualValues(t, 1, v)

	defer test.MockVariableValue(&timeNow, func() time.Time {
		return time.Now().Add(5 * time.Minute)
	})()
	v, _ = c.Get(field, "my_config1")
	assert.Nil(t, v)
}

func TestContextCacheDeleteGroup(t *testing.T) {
	ctx := WithCacheContext(t.Context())
	c := GetContextCache(ctx)
	c.Put("group", 1, "a")
	c.Put("group", 2, "b")
	c.Put("other", 1, "c")

	c.DeleteGroup("group")
	_, has := c.Get("group", 1)
	assert.False(t, has)
	_, has = c.Get("group", 2)
	assert.False(t, has)
	v, has := c.Get("other", 1)
	assert.True(t, has)
	assert.Equal(t, "c", v)
}
