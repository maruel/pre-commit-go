// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"testing"

	"github.com/maruel/pre-commit-go/checks"
	"github.com/maruel/ut"
)

func TestProcessModes(t *testing.T) {
	data := []struct {
		in       string
		expected []checks.Mode
		err      error
	}{
		{"all", []checks.Mode{checks.ContinuousIntegration, checks.Lint}, nil},
		{"", nil, nil},
		{"pc", []checks.Mode{checks.PreCommit}, nil},
		{"fast", []checks.Mode{checks.PreCommit}, nil},
		{"pp", []checks.Mode{checks.PrePush}, nil},
		{"slow", []checks.Mode{checks.PrePush}, nil},
		{"ci", []checks.Mode{checks.ContinuousIntegration}, nil},
		{"full", []checks.Mode{checks.ContinuousIntegration}, nil},
		{"foo", nil, errors.New("invalid mode \"foo\"\n\n" + helpModes)},
	}
	for i, line := range data {
		actual, err := processModes(line.in)
		ut.AssertEqualIndex(t, i, line.expected, actual)
		ut.AssertEqualIndex(t, i, line.err, err)
	}
}
