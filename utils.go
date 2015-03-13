// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// capture runs an executable and returns the output, exit code and error if
// appropriate.
func capture(args ...string) (string, int, error) {
	exitCode := -1
	log.Printf("capture(%s)", args)
	c := exec.Command(args[0], args[1:]...)
	out, err := c.CombinedOutput()
	if c.ProcessState != nil {
		if waitStatus, ok := c.ProcessState.Sys().(syscall.WaitStatus); ok {
			exitCode = waitStatus.ExitStatus()
			if exitCode != 0 {
				err = nil
			}
		}
	}
	// TODO(maruel): Handle code page on Windows.
	return string(out), exitCode, err
}

// captureAbs returns an absolute path of whatever a git command returned.
func captureAbs(args ...string) (string, error) {
	out, code, _ := capture(args...)
	if code != 0 {
		return "", fmt.Errorf("failed to run \"%s\"", strings.Join(args, " "))
	}
	path, err := filepath.Abs(strings.TrimSpace(out))
	log.Printf("captureAbs(%s) = %s", args, path)
	return path, err
}
