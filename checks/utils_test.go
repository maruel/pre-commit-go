// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/ut"
)

func TestIsMainPackage(t *testing.T) {
	t.Parallel()
	data := []struct {
		expected string
		in       string
	}{
		{"foo", "// Hi\npackage foo\n"},
		{"main", "package main\n"},
		{"", ""},
	}
	for i, line := range data {
		ut.AssertEqualIndex(t, i, line.expected, getPackageName([]byte(line.in)))
	}
}

func TestInternalCheck(t *testing.T) {
	t.Parallel()
	d, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "checks", filepath.Base(d))
}

func TestGoDirs(t *testing.T) {
	defer func() {
		goDirsCache = nil
	}()

	checksDir, _ := os.Getwd()
	preCommitGoDir := filepath.Dir(checksDir)
	defer func() {
		_ = os.Chdir(checksDir)
	}()
	internalDir := filepath.Join(preCommitGoDir, "internal")
	scmDir := filepath.Join(preCommitGoDir, "scm")
	ut.AssertEqual(t, nil, os.Chdir(preCommitGoDir))
	ut.AssertEqual(t, []string{preCommitGoDir, checksDir, internalDir, scmDir}, goDirs(sourceDirs))
	ut.AssertEqual(t, []string{checksDir, scmDir}, goDirs(testDirs))
	ut.AssertEqual(t, []string{checksDir, internalDir, scmDir}, goDirs(packageDirs))
}
