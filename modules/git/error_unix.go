// Copyright 2025 okTurtles Foundation. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package git

import (
	"errors"
	"os/exec"
	"syscall"
)

// isExitErrorKilled checks if the error is an exec.ExitError caused by SIGKILL.
// On Unix systems, we can properly check the signal that terminated the process.
func isExitErrorKilled(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return false
	}

	// Check if the process was terminated by a signal (specifically SIGKILL)
	return waitStatus.Signaled() && waitStatus.Signal() == syscall.SIGKILL
}
