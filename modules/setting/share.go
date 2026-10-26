// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import "strings"

// What a link to Forkana shows when it is shared (the og: and twitter: tags of
// templates/base/head_opengraph.tmpl, #450).
const (
	// ForkanaName is the site name shared links show when APP_NAME is unset or still Gitea's.
	ForkanaName = "Forkana"
	// ForkanaDescription is the site description shared links (and the description meta
	// tag) show when [ui.meta] DESCRIPTION is unset or still Gitea's.
	ForkanaDescription = "Forkana is a new kind of online encyclopedia: read, fork and improve articles on any subject, and compare their versions."

	// ShareImagePath is the image every shared link shows, under the asset url prefix
	// (custom/public/assets/img/forkana-share.png).
	ShareImagePath   = "/img/forkana-share.png"
	ShareImageWidth  = 1200
	ShareImageHeight = 630
	ShareImageAlt    = "Forkana, a new kind of online encyclopedia"
)

// isGiteaDefault reports whether a configured value is empty or still one of Gitea's own
// ("Gitea: Git with a cup of tea", "Gitea (Git with a cup of tea) is a painless ...").
func isGiteaDefault(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.Contains(strings.ToLower(s), "gitea")
}

// ShareSiteName is the site name shared links show: APP_NAME, unless it is unset or
// still Gitea's, in which case Forkana.
func ShareSiteName() string {
	if isGiteaDefault(AppName) {
		return ForkanaName
	}
	return AppName
}

// ShareSiteDescription is the site description shared links show: [ui.meta] DESCRIPTION,
// unless it is unset or still Gitea's, in which case Forkana's.
func ShareSiteDescription() string {
	if isGiteaDefault(UI.Meta.Description) {
		return ForkanaDescription
	}
	return UI.Meta.Description
}
