// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"

	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/storage"
	attachment_service "code.gitea.io/gitea/services/attachment"

	"github.com/urfave/cli/v3"
)

var microcmdBackfillArticleAttachments = &cli.Command{
	Name:  "backfill-article-attachments",
	Usage: "Record the article attachment associations of repositories that predate the association table",
	Description: `Scans the retained article history of every repository and records which attachments it keeps alive.

The run is idempotent and restartable, so it is safe to run it again after a failure or an interruption; use --start-repo-id to resume where the previous run stopped.

Once a --verify run reports nothing outstanding, --finalize switches attachment authorization to associations only. Enable the gc_article_attachments cron task afterwards, never before.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Scan and report without writing any association",
		},
		&cli.BoolFlag{
			Name:  "verify",
			Usage: "Report whether the legacy read fallback can be switched off, without writing anything",
		},
		&cli.BoolFlag{
			Name:  "finalize",
			Usage: "Verify readiness and then disable the legacy read fallback, or refuse and report what is outstanding",
		},
		&cli.IntFlag{
			Name:  "batch-size",
			Usage: "How many repositories to load at a time",
			Value: attachment_service.DefaultBackfillBatchSize,
		},
		&cli.Int64Flag{
			Name:  "start-repo-id",
			Usage: "Resume at this repository ID",
		},
	},
	Action: runBackfillArticleAttachments,
}

func runBackfillArticleAttachments(ctx context.Context, c *cli.Command) error {
	finalize := c.Bool("finalize")
	if finalize && (c.Bool("dry-run") || c.Bool("verify")) {
		return errors.New("--finalize cannot be combined with --dry-run or --verify; it verifies on its own")
	}

	if err := initDB(ctx); err != nil {
		return err
	}
	if err := git.InitSimple(); err != nil {
		return err
	}
	if err := storage.Init(); err != nil {
		return err
	}

	opts := attachment_service.BackfillOptions{
		DryRun:      c.Bool("dry-run"),
		Verify:      c.Bool("verify"),
		BatchSize:   c.Int("batch-size"),
		StartRepoID: c.Int64("start-repo-id"),
	}

	var result *attachment_service.BackfillResult
	var err error
	if finalize {
		result, err = attachment_service.FinalizeLegacyFallback(ctx, opts)
	} else {
		result, err = attachment_service.BackfillArticleAttachments(ctx, opts)
	}
	if result != nil {
		printBackfillArticleAttachmentsResult(result)
	}
	if err != nil {
		return err
	}
	if finalize {
		fmt.Println("The legacy article attachment read fallback is now disabled.")
	}
	return nil
}

func printBackfillArticleAttachmentsResult(result *attachment_service.BackfillResult) {
	fmt.Printf(`repositories scanned:            %d
unreadable repositories:         %d
references found:                %d
associations inserted:           %d
legacy rows inferred:            %d
suspicious references skipped:   %d
missing attachments:             %d
missing stored objects:          %d
outstanding references:          %d
unassociated legacy attachments: %d
last repository processed:       %d
`,
		result.ReposScanned, result.RepositoriesFailed, result.ReferencesFound, result.AssociationsInserted,
		result.LegacyInferred, result.SuspiciousSkipped, result.MissingAttachments, result.MissingFiles,
		result.Outstanding, result.UnassociatedLegacy, result.LastRepoID)
}
