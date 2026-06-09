// Copyright 2017 The Gitea Authors. All rights reserved.
// Copyright 2017 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package markup

import (
	"net/url"
	"regexp"
	"strings"
	"sync"

	"code.gitea.io/gitea/modules/setting"

	"github.com/microcosm-cc/bluemonday"
)

// Sanitizer is a protection wrapper of *bluemonday.Policy which does not allow
// any modification to the underlying policies once it's been created.
type Sanitizer struct {
	defaultPolicy     *bluemonday.Policy
	descriptionPolicy *bluemonday.Policy
	rendererPolicies  map[string]*bluemonday.Policy
	allowAllRegex     *regexp.Regexp
}

var (
	defaultSanitizer     *Sanitizer
	defaultSanitizerOnce sync.Once
)

func GetDefaultSanitizer() *Sanitizer {
	defaultSanitizerOnce.Do(func() {
		defaultSanitizer = &Sanitizer{
			rendererPolicies: map[string]*bluemonday.Policy{},
			allowAllRegex:    regexp.MustCompile(".+"),
		}
		for name, renderer := range renderers {
			sanitizerRules := renderer.SanitizerRules()
			if len(sanitizerRules) > 0 {
				policy := defaultSanitizer.createDefaultPolicy()
				defaultSanitizer.addSanitizerRules(policy, sanitizerRules)
				defaultSanitizer.rendererPolicies[name] = policy
			}
		}
		defaultSanitizer.defaultPolicy = defaultSanitizer.createDefaultPolicy()
		defaultSanitizer.descriptionPolicy = defaultSanitizer.createRepoDescriptionPolicy()
	})
	return defaultSanitizer
}

func ResetDefaultSanitizerForTesting() {
	defaultSanitizer = nil
	defaultSanitizerOnce = sync.Once{}
}

func allowDataURIImagesPolicy(u *url.URL) bool {
	// Enforce a size limit based on MaxDisplayFileSize (plus base64 overhead) to protect against storage bloat and slow decoding
	maxSize := setting.UI.MaxDisplayFileSize * 4 / 3
	if maxSize <= 0 {
		maxSize = 20 * 1024 * 1024 * 4 / 3
	}
	if int64(len(u.Opaque)) > maxSize {
		return false
	}
	// Replicate bluemonday's strict validation (Issue 2)
	// It must start with one of the allowed mime types followed by ;base64,
	parts := strings.SplitN(u.Opaque, ";base64,", 2)
	if len(parts) != 2 {
		return false
	}
	mimeType := parts[0]
	if mimeType != "image/gif" && mimeType != "image/jpeg" && mimeType != "image/png" &&
		mimeType != "image/webp" && mimeType != "image/svg+xml" {
		return false
	}
	// Validate that the remaining data is valid base64 (only alphanumeric, +, /, and =)
	payload := parts[1]
	if len(payload) == 0 {
		return false
	}
	for _, char := range payload {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') || char == '+' || char == '/' || char == '=' ||
			char == '\r' || char == '\n' || char == '\t' || char == ' ') {
			return false
		}
	}
	return true
}
