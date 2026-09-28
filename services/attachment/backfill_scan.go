// Copyright 2026 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

package attachment

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/markup/attachmentref"
)

// defaultMaxHistoryBlobs caps how many distinct article blobs one repository
// contributes to a history scan. A long-lived article can hold thousands of
// revisions, and the oldest ones are the least likely to hold a reference no
// later revision has.
//
// A capped scan is incomplete and nothing repairs it later: reconciliation
// runs the same scan under the same cap. A run that reaches the cap therefore
// reports the repository, and finalizing refuses until a higher cap has read
// the whole history.
const defaultMaxHistoryBlobs = 4096

// scanArticleHistoryUUIDs returns the attachment UUIDs referenced by any
// retained version of the article content of the repository at repoPath, and
// whether the scan stopped at maxBlobs before reaching the end of the history.
//
// History, not just the tip, is what matters: an association outlives the
// content that introduced it, so a reference that only an older commit still
// carries must keep its attachment alive.
func scanArticleHistoryUUIDs(ctx context.Context, repoPath string, maxBlobs int) ([]string, bool, error) {
	shas, capped, err := articleBlobSHAs(ctx, repoPath, maxBlobs)
	if err != nil || len(shas) == 0 {
		return nil, capped, err
	}
	uuids, err := uuidsFromBlobs(ctx, repoPath, shas)
	return uuids, capped, err
}

// articleBlobSHAs lists the distinct blobs that ever appeared under an article
// content path, and reports whether it stopped at the cap. One rev-list walk
// per repository replaces the object-per-commit lookups a tree traversal would
// need.
func articleBlobSHAs(ctx context.Context, repoPath string, maxBlobs int) ([]string, bool, error) {
	if maxBlobs <= 0 {
		maxBlobs = defaultMaxHistoryBlobs
	}
	stdoutReader, stdoutWriter := io.Pipe()
	stderr := new(bytes.Buffer)
	go func() {
		err := gitcmd.NewCommand("rev-list", "--objects", "--all").Run(ctx, &gitcmd.RunOpts{
			Dir:    repoPath,
			Stdout: stdoutWriter,
			Stderr: stderr,
		})
		if err != nil {
			err = fmt.Errorf("git rev-list --objects --all [%s]: %w - %s", repoPath, err, stderr.String())
		}
		_ = stdoutWriter.CloseWithError(err)
	}()
	// Closing the read end stops git when the cap is reached.
	defer stdoutReader.Close()

	shas := make([]string, 0, 8)
	seen := make(container.Set[string], 8)
	capped := false
	scanner := bufio.NewScanner(stdoutReader)
	for scanner.Scan() {
		// Commits and trees are listed without a path, so only named objects
		// reach the article-path test.
		sha, path, named := strings.Cut(scanner.Text(), " ")
		if !named || !isArticlePath(path) {
			continue
		}
		if seen.Add(sha) {
			shas = append(shas, sha)
		}
		if len(shas) >= maxBlobs {
			capped = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, capped, err
	}
	return shas, capped, nil
}

// uuidsFromBlobs reads the given blobs through a single cat-file batch and
// extracts the attachment references they carry.
//
// git.Repository.CatFileBatch is deliberately not used here: it exists only in
// the non-gogit build, and this code has to compile in both.
func uuidsFromBlobs(ctx context.Context, repoPath string, shas []string) ([]string, error) {
	stdoutReader, stdoutWriter := io.Pipe()
	stderr := new(bytes.Buffer)
	go func() {
		err := gitcmd.NewCommand("cat-file", "--batch").Run(ctx, &gitcmd.RunOpts{
			Dir:    repoPath,
			Stdin:  strings.NewReader(strings.Join(shas, "\n") + "\n"),
			Stdout: stdoutWriter,
			Stderr: stderr,
		})
		if err != nil {
			err = fmt.Errorf("git cat-file --batch [%s]: %w - %s", repoPath, err, stderr.String())
		}
		_ = stdoutWriter.CloseWithError(err)
	}()
	defer stdoutReader.Close()

	batchReader := bufio.NewReaderSize(stdoutReader, 32*1024)
	uuids := make([]string, 0, 8)
	seen := make(container.Set[string], 8)
	for range shas {
		_, typ, size, err := git.ReadBatchLine(batchReader)
		if err != nil {
			if errors.Is(err, io.EOF) || git.IsErrNotExist(err) {
				break
			}
			return nil, err
		}
		if typ != "blob" {
			if err := git.DiscardFull(batchReader, size+1); err != nil {
				return nil, err
			}
			continue
		}
		content, err := readBlobContent(batchReader, size)
		if err != nil {
			return nil, err
		}
		for _, attachmentUUID := range attachmentref.ExtractAttachmentUUIDs(content) {
			if seen.Add(attachmentUUID) {
				uuids = append(uuids, attachmentUUID)
			}
		}
	}
	return uuids, nil
}

// readBlobContent reads one batch entry, keeping at most the scannable prefix
// and discarding the remainder so the next header stays aligned.
func readBlobContent(rd *bufio.Reader, size int64) (string, error) {
	kept := min(size, int64(attachmentref.MaxScanSize))
	buf := make([]byte, kept)
	if _, err := io.ReadFull(rd, buf); err != nil {
		return "", err
	}
	if err := git.DiscardFull(rd, size-kept+1); err != nil {
		return "", err
	}
	return string(buf), nil
}

// isArticlePath reports whether a history path holds article content. The match
// is case-insensitive because the reader renders whichever case variant a
// repository happens to carry, so a scan that only accepted the canonical
// spelling would miss references the article still displays.
func isArticlePath(path string) bool {
	return slices.ContainsFunc(articleContentPaths, func(candidate string) bool {
		return strings.EqualFold(candidate, path)
	})
}
