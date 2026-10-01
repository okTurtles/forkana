// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/services/context"
)

// RedirectLegacyArticle permanently redirects the retired "/article/{username}/{subjectname}[/*]"
// urls to "/subject/{subjectname}/{username}[/*]", so links to the old scheme keep working.
// The old scheme had no article index, so they land on the owner's current article.
func RedirectLegacyArticle(ctx *context.Context) {
	// the raw params are still escaped as requested, so they are reassembled as they are
	link := setting.AppSubURL + "/subject/" + ctx.PathParamRaw("subjectname") + "/" + ctx.PathParamRaw("username")
	if rest := ctx.PathParamRaw("*"); rest != "" {
		link += "/" + rest
	}
	redirectLegacyArticle(ctx, link)
}

// RedirectLegacyArticleRepo permanently redirects the retired "/article/repo/{username}/{reponame}"
// url to the permanent article url of the repository, "/{username}/{reponame}".
func RedirectLegacyArticleRepo(ctx *context.Context) {
	redirectLegacyArticle(ctx, setting.AppSubURL+"/"+ctx.PathParamRaw("username")+"/"+ctx.PathParamRaw("reponame"))
}

func redirectLegacyArticle(ctx *context.Context, link string) {
	if ctx.Req.URL.RawQuery != "" {
		link += "?" + ctx.Req.URL.RawQuery
	}
	ctx.Redirect(link, http.StatusMovedPermanently)
}
