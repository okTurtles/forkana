// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"errors"
	"net/http"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/services/citation"
	"code.gitea.io/gitea/services/context"
)

// CitationLookup returns, as JSON, the details of the source the "q" form value names (a
// URL, a DOI or an ISBN), for the Cite dialog of the article editor (issue #399). It is a
// POST, so the CSRF check keeps other sites from making a signed-in user's browser trigger
// fetches; lookups are rate-limited per user and cached (services/citation). When nothing
// can be found the response is 404 and the editor lets the writer fill in the citation.
func CitationLookup(ctx *context.Context) {
	query := ctx.FormTrim("q")
	src, err := citation.LookupFor(ctx, ctx.Doer.ID, query)
	if err != nil {
		switch {
		// a site redirecting somewhere a lookup may not go is "not found", not a bad query
		case errors.Is(err, citation.ErrUnsupported) && !errors.Is(err, citation.ErrRedirectRefused):
			ctx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, citation.ErrRateLimited):
			ctx.JSON(http.StatusTooManyRequests, map[string]string{"error": err.Error()})
		default:
			log.Debug("CitationLookup(%q): %v", query, err)
			ctx.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}
		return
	}
	ctx.JSON(http.StatusOK, src)
}
