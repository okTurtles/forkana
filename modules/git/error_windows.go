// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package git

// isExitErrorKilled checks if the error is an exec.ExitError caused by a kill signal.
// On Windows, we cannot reliably detect SIGKILL since Windows doesn't have Unix signals.
// Windows always returns exit code 1 when a process is terminated, so we fall back to
// string matching as a best-effort approach.
func isExitErrorKilled(err error) bool {
	// Windows doesn't have proper signal semantics, so we use string matching as fallback.
	// This is fragile but unavoidable on Windows.
	return err != nil && err.Error() == "signal: killed"
}
