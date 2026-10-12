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

// CitationLookup returns, as JSON, the details of the source the "q" query names (a URL, a
// DOI or an ISBN), for the Cite dialog of the article editor (issue #399). When nothing can
// be found the response is 404 and the editor lets the writer fill in the citation.
func CitationLookup(ctx *context.Context) {
	query := ctx.FormTrim("q")
	src, err := citation.Lookup(ctx, query)
	if err != nil {
		if errors.Is(err, citation.ErrUnsupported) {
			ctx.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		log.Debug("CitationLookup(%q): %v", query, err)
		ctx.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	ctx.JSON(http.StatusOK, src)
}
