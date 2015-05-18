// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"log"
	"os/exec"
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
