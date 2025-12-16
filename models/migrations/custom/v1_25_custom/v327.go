// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_25_custom

import (
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// repositoryForkOnEditIndexes is a temporary struct used only for this migration.
// It defines composite indexes to optimize fork-on-edit permission queries:
// - IDX_repository_owner_subject: for GetRepositoryByOwnerIDAndSubjectID()
// - IDX_repository_owner_fork: for GetForkedRepo()
type repositoryForkOnEditIndexes struct {
	ID        int64 `xorm:"pk autoincr"`
	OwnerID   int64
	SubjectID int64
	ForkID    int64
}

func (*repositoryForkOnEditIndexes) TableName() string {
	return "repository"
}

func (*repositoryForkOnEditIndexes) TableIndices() []*schemas.Index {
	// Composite index for GetRepositoryByOwnerIDAndSubjectID()
	// Query: WHERE owner_id = ? AND subject_id = ?
	ownerSubjectIndex := schemas.NewIndex("IDX_repository_owner_subject", schemas.IndexType)
	ownerSubjectIndex.AddColumn("owner_id", "subject_id")

	// Composite index for GetForkedRepo()
	// Query: WHERE owner_id = ? AND fork_id = ?
	ownerForkIndex := schemas.NewIndex("IDX_repository_owner_fork", schemas.IndexType)
	ownerForkIndex.AddColumn("owner_id", "fork_id")

	return []*schemas.Index{ownerSubjectIndex, ownerForkIndex}
}

// AddCompositeIndexesForForkOnEdit adds composite indexes to optimize fork-on-edit permission queries.
func AddCompositeIndexesForForkOnEdit(x *xorm.Engine) error {
	return x.Sync(new(repositoryForkOnEditIndexes))
}
