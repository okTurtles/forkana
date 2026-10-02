// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package explore

import (
	"net/http"
	"slices"
	"strings"

	"code.gitea.io/gitea/models/db"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/services/context"
)

// subjectSuggestionLimit is how many subjects the search field suggests at most.
const subjectSuggestionLimit = 5

// SubjectSuggestions returns the subjects matching the "q" keyword as JSON, for the suggestion
// dropdown of the landing page search field. It mirrors what Subjects() renders for the same
// keyword: the exact match first (if there is one), then the most similar subjects.
func SubjectSuggestions(ctx *context.Context) {
	keyword := ctx.FormTrim("q")
	if keyword == "" {
		ctx.JSON(http.StatusOK, map[string]any{"subjects": []string{}})
		return
	}

	names := make([]string, 0, subjectSuggestionLimit)

	exactSubjects, _, err := repo_model.FindSubjects(ctx, repo_model.FindSubjectsOptions{
		ListOptions:    db.ListOptions{Page: 1, PageSize: 1},
		Keyword:        keyword,
		ExactMatchOnly: true,
	})
	if err != nil {
		ctx.ServerError("FindSubjects (exact)", err)
		return
	}

	excludeIDs := make([]int64, 0, 1)
	if len(exactSubjects) > 0 {
		excludeIDs = append(excludeIDs, exactSubjects[0].ID)
		names = append(names, exactSubjects[0].Name)
	}

	similarSubjects, err := repo_model.FindSimilarSubjects(ctx, repo_model.FindSimilarSubjectsOptions{
		Keyword:    keyword,
		Limit:      subjectSuggestionLimit - len(names),
		ExcludeIDs: excludeIDs,
		OrderBy:    repo_model.SubjectOrderBy(repo_model.SubjectSortAlphabetically),
	})
	if err != nil {
		ctx.ServerError("FindSimilarSubjects", err)
		return
	}
	similarNames := make([]string, len(similarSubjects))
	for i, subject := range similarSubjects {
		similarNames[i] = subject.Name
	}

	// FindSimilarSubjects picks the matches by similarity but hands them back in the requested
	// display order, which for a type-ahead buries the subjects that start with what has just
	// been typed. Lift those back up, keeping the alphabetical order within each group.
	prefix := strings.ToLower(keyword)
	slices.SortStableFunc(similarNames, func(a, b string) int {
		aStarts, bStarts := strings.HasPrefix(strings.ToLower(a), prefix), strings.HasPrefix(strings.ToLower(b), prefix)
		switch {
		case aStarts == bStarts:
			return 0
		case aStarts:
			return -1
		default:
			return 1
		}
	})
	names = append(names, similarNames...)

	ctx.JSON(http.StatusOK, map[string]any{"subjects": names})
}
