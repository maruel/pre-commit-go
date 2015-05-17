// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// CaptureWd runs an executable from a directory returns the output, exit code
// and error if appropriate.
func CaptureWd(wd string, args ...string) (string, int, error) {
	exitCode := -1
	log.Printf("CaptureWd(%s, %s)", wd, args)
	c := exec.Command(args[0], args[1:]...)
	if wd != "" {
		c.Dir = wd
	}
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

// Capture runs an executable and returns the output, exit code and error if
// appropriate.
func Capture(args ...string) (string, int, error) {
	return CaptureWd("", args...)
}

// CaptureAbs returns an absolute path of whatever a git command returned.
func CaptureAbs(args ...string) (string, error) {
	out, code, _ := Capture(args...)
	if code != 0 {
		return "", fmt.Errorf("failed to run \"%s\"", strings.Join(args, " "))
	}
	path, err := filepath.Abs(strings.TrimSpace(out))
	log.Printf("CaptureAbs(%s) = %s", args, path)
	return path, err
}
