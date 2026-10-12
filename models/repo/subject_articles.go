// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	"code.gitea.io/gitea/models/db"

	"xorm.io/builder"
)

// publicArticleCond selects the articles a reader can open from a subject: public, not
// deleted (tombstones), with content.
func publicArticleCond() builder.Cond {
	return builder.Eq{
		"repository.is_private":    false,
		"repository.is_tombstoned": false,
		"repository.is_empty":      false,
	}
}

// CountPublicArticlesBySubjectIDs returns, for each of the subjects, how many articles a
// reader can open in it (see publicArticleCond); subjects without any are left out.
func CountPublicArticlesBySubjectIDs(ctx context.Context, subjectIDs []int64) (map[int64]int64, error) {
	counts := make(map[int64]int64, len(subjectIDs))
	if len(subjectIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		SubjectID int64
		Count     int64
	}
	err := db.GetEngine(ctx).Table("repository").
		Select("subject_id, COUNT(*) AS count").
		Where(builder.In("subject_id", subjectIDs).And(publicArticleCond())).
		GroupBy("subject_id").
		Find(&rows)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.SubjectID] = r.Count
	}
	return counts, nil
}

// FindPublicArticlesBySubjectID returns the articles a reader can open in the subject (see
// publicArticleCond), the most recently updated first. Their owners are not loaded.
func FindPublicArticlesBySubjectID(ctx context.Context, subjectID int64) (RepositoryList, error) {
	var repos RepositoryList
	err := db.GetEngine(ctx).Table("repository").
		Where(builder.Eq{"repository.subject_id": subjectID}.And(publicArticleCond())).
		OrderBy(articleOrderBy).
		Find(&repos)
	return repos, err
}
