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
		names = append(names, exactSubjects[0].Name)
		subjects = append(subjects, exactSubjects[0])
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
	for _, subject := range similarSubjects {
		names = append(names, subject.Name)
		subjects = append(subjects, subject)
	}

	resp := map[string]any{"subjects": names}
	if ctx.FormBool("detail") {
		// the link-to-subject search of the article editor (issue #399) shows how many
		// articles each subject holds
		ids := make([]int64, len(subjects))
		for i, subject := range subjects {
			ids[i] = subject.ID
		}
		counts, err := repo_model.CountPublicArticlesBySubjectIDs(ctx, ids)
		if err != nil {
			ctx.ServerError("CountPublicArticlesBySubjectIDs", err)
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

// SubjectArticles returns, as JSON, the owners of the articles of the "subject" a reader can
// open, the most recently updated first: the link-to-subject search of the article editor
// lists them to link to one author's version of the subject (issue #399).
func SubjectArticles(ctx *context.Context) {
	name := ctx.FormTrim("subject")
	subject, err := repo_model.GetSubjectByName(ctx, name)
	if err != nil {
		if repo_model.IsErrSubjectNotExist(err) {
			ctx.JSON(http.StatusOK, map[string]any{"articles": []any{}})
			return
		}
		ctx.ServerError("GetSubjectByName", err)
		return
	}
	repos, err := repo_model.FindPublicArticlesBySubjectID(ctx, subject.ID)
	if err != nil {
		ctx.ServerError("FindPublicArticlesBySubjectID", err)
		return
	}
	type article struct {
		Owner string `json:"owner"`
	}
	articles := make([]article, 0, len(repos))
	seen := make(map[string]bool, len(repos))
	for _, repo := range repos {
		// an owner's archived articles are older versions: link to the owner, whose url
		// shows the current one
		if seen[repo.OwnerName] {
			continue
		}
		seen[repo.OwnerName] = true
		articles = append(articles, article{Owner: repo.OwnerName})
	}
	ctx.JSON(http.StatusOK, map[string]any{"subject": subject.Name, "articles": articles})
}
