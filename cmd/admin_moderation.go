// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"

	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/storage"
	moderation_service "code.gitea.io/gitea/services/moderation"

	"github.com/urfave/cli/v3"
)

var subcmdModeration = &cli.Command{
	Name:  "moderation",
	Usage: "Manage moderation",
	Commands: []*cli.Command{
		microcmdModerationSetup,
	},
}

var microcmdModerationSetup = &cli.Command{
	Name:  "setup",
	Usage: "Create the moderation organization, team, private cases repository and labels that are missing",
	Description: `Creates what moderation needs and doesn't exist yet: the organization ([moderation] ORG), its moderators team ([moderation] TEAM, write access to issues), the private cases repository ([moderation] REPO, issues only) and the core labels, as organization labels.

What already exists is left as it is, so the command is safe to run again, also after a failed run. It works whether [moderation] ENABLED is on or not, so moderation can be prepared before it is enabled.

It finishes by removing every watch on the cases repository, so moderators aren't notified of every report.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "owner",
			Usage: "Username of the site administrator who owns the organization and creates the repository (default: the first site administrator)",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Report what would be created without writing anything",
		},
	},
	Action: runModerationSetup,
}

func runModerationSetup(ctx context.Context, c *cli.Command) error {
	if err := initDB(ctx); err != nil {
		return err
	}
	if err := git.InitSimple(); err != nil {
		return err
	}
	// Creating the organization saves its generated avatar.
	if err := storage.Init(); err != nil {
		return err
	}

	var owner *user_model.User
	var err error
	if name := c.String("owner"); name != "" {
		if owner, err = user_model.GetUserByName(ctx, name); err != nil {
			return fmt.Errorf("get owner %q: %w", name, err)
		}
	} else if owner, err = user_model.GetAdminUser(ctx); err != nil && !user_model.IsErrUserNotExist(err) {
		return fmt.Errorf("get a site administrator: %w", err)
	}

	result, err := moderation_service.EnsureSetup(ctx, moderation_service.SetupOptions{
		Owner:  owner,
		DryRun: c.Bool("dry-run"),
	})
	if err != nil {
		return err
	}

	created := "created"
	if c.Bool("dry-run") {
		created = "would be created"
	}
	for _, it := range result.Items {
		status := "already present"
		if it.Created {
			status = created
		}
		fmt.Printf("%-16s %-40s %s\n", it.Kind, it.Name, status)
	}
	if !c.Bool("dry-run") {
		fmt.Printf("watches removed: %d\n", result.Unwatched)
	}
	if !setting.Moderation.Enabled {
		fmt.Println("Moderation is off: set [moderation] ENABLED = true to turn it on.")
	}
	return nil
}
