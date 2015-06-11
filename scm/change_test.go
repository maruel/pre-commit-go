// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/ut"
)

func TestInternalCheck(t *testing.T) {
	t.Parallel()
	d, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "scm", filepath.Base(d))
}

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
