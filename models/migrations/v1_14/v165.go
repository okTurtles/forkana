// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1_14

import (
	"code.gitea.io/gitea/models/migrations/base"

	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

func ConvertHookTaskTypeToVarcharAndTrim(x *xorm.Engine) error {
	dbType := x.Dialect().URI().DBType
	if dbType == schemas.SQLITE { // For SQLITE, varchar or char will always be represented as TEXT
		return nil
	}

	// HookTask: Typ string `xorm:"VARCHAR(16) index"`
	if err := base.ModifyColumn(x, "hook_task", &schemas.Column{
		Name: "typ",
		SQLType: schemas.SQLType{
			Name: "VARCHAR",
		},
		Length:         16,
		Nullable:       true, // To keep compatible as nullable
		DefaultIsEmpty: true,
	}); err != nil {
		return err
	}

	if _, err := x.Exec("UPDATE hook_task SET typ = TRIM(typ)"); err != nil {
		return err
	}

	// Webhook: Type string `xorm:"VARCHAR(16) index"`
	if err := base.ModifyColumn(x, "webhook", &schemas.Column{
		Name: "type",
		SQLType: schemas.SQLType{
			Name: "VARCHAR",
		},
		Length:         16,
		Nullable:       true, // To keep compatible as nullable
		DefaultIsEmpty: true,
	}); err != nil {
		return err
	}

	_, err := x.Exec("UPDATE webhook SET type = TRIM(type)")
	return err
}
