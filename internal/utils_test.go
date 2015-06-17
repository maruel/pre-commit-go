// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/maruel/ut"
)

func TestCaptureNormal(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	out, code, err := Capture(wd, []string{"FOO=BAR"}, "go", "version")
	ut.AssertEqual(t, true, strings.Contains(out, runtime.Version()))
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)
}

func TestCaptureEmpty(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	out, code, err := Capture(wd, nil)
	ut.AssertEqual(t, "", out)
	ut.AssertEqual(t, -1, code)
	ut.AssertEqual(t, errors.New("no command specified"), err)
}

func TestCaptureOne(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	_, code, err := Capture(wd, nil, "go")
	ut.AssertEqual(t, 2, code)
	ut.AssertEqual(t, nil, err)
}

func TestCaptureMissing(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	out, code, err := Capture(wd, nil, "program_is_non_existent")
	ut.AssertEqual(t, "", out)
	ut.AssertEqual(t, -1, code)
	ut.AssertEqual(t, true, err != nil)
}

func TestCaptureNoWd(t *testing.T) {
	t.Parallel()
	_, code, err := Capture("", nil, "go")
	ut.AssertEqual(t, -1, code)
	ut.AssertEqual(t, errors.New("wd is required"), err)
}
