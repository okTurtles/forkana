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

	subjects := make([]*repo_model.Subject, 0, subjectSuggestionLimit)

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
		subjects = append(subjects, exactSubjects[0])
	}

	similarSubjects, err := repo_model.FindSimilarSubjects(ctx, repo_model.FindSimilarSubjectsOptions{
		Keyword:    keyword,
		Limit:      subjectSuggestionLimit - len(subjects),
		ExcludeIDs: excludeIDs,
		OrderBy:    repo_model.SubjectOrderBy(repo_model.SubjectSortAlphabetically),
	})
	if err != nil {
		ctx.ServerError("FindSimilarSubjects", err)
		return
	}

	// FindSimilarSubjects picks the matches by similarity but hands them back in the requested
	// display order, which for a type-ahead buries the subjects that start with what has just
	// been typed. Lift those back up, keeping the alphabetical order within each group.
	prefix := strings.ToLower(keyword)
	slices.SortStableFunc(similarSubjects, func(a, b *repo_model.Subject) int {
		aStarts, bStarts := strings.HasPrefix(strings.ToLower(a.Name), prefix), strings.HasPrefix(strings.ToLower(b.Name), prefix)
		switch {
		case aStarts == bStarts:
			return 0
		case aStarts:
			return -1
		default:
			return 1
		}
	})
	subjects = append(subjects, similarSubjects...)

	names := make([]string, len(subjects))
	for i, subject := range subjects {
		names[i] = subject.Name
	}
	resp := map[string]any{"subjects": names}
	if ctx.FormBool("detail") {
		// the link-to-subject search of the article editor (issue #399) shows how many
		// articles each subject holds
		ids := make([]int64, len(subjects))
		for i, subject := range subjects {
			ids[i] = subject.ID
		}
		counts, err := repo_model.CountReadableArticlesBySubjectIDs(ctx, ids, ctx.Doer)
		if err != nil {
			ctx.ServerError("CountReadableArticlesBySubjectIDs", err)
			return
		}
		details := make([]subjectSuggestionDetail, len(subjects))
		for i, subject := range subjects {
			details[i] = subjectSuggestionDetail{Name: subject.Name, Articles: counts[subject.ID]}
		}
		resp["details"] = details
	}
	ctx.JSON(http.StatusOK, resp)
}

// subjectSuggestionDetail is a suggested subject with the number of its articles.
type subjectSuggestionDetail struct {
	Name     string `json:"name"`
	Articles int64  `json:"articles"`
}

// subjectArticle is an article listed by SubjectArticles.
type subjectArticle struct {
	Owner string `json:"owner"`
}

// SubjectArticles returns, as JSON, the owners of the articles of the "subject" a reader can
// open, the most recently updated first: the link-to-subject search of the article editor
// lists them to link to one author's version of the subject (issue #399). The response has
// the same shape for an unknown subject, with "subject" empty.
func SubjectArticles(ctx *context.Context) {
	name := ctx.FormTrim("subject")
	if name == "" {
		// GetSubjectByName("") has no condition and would return any subject
		ctx.JSON(http.StatusOK, map[string]any{"subject": "", "articles": []subjectArticle{}})
		return
	}
	subject, err := repo_model.GetSubjectByName(ctx, name)
	if err != nil {
		if repo_model.IsErrSubjectNotExist(err) {
			ctx.JSON(http.StatusOK, map[string]any{"subject": "", "articles": []subjectArticle{}})
			return
		}
		ctx.ServerError("GetSubjectByName", err)
		return
	}
	owners, err := repo_model.FindReadableArticleOwners(ctx, subject.ID, ctx.Doer)
	if err != nil {
		ctx.ServerError("FindReadableArticleOwners", err)
		return
	}
	articles := make([]subjectArticle, len(owners))
	for i, owner := range owners {
		articles[i] = subjectArticle{Owner: owner}
	}
	ctx.JSON(http.StatusOK, map[string]any{"subject": subject.Name, "articles": articles})
}
