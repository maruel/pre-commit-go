// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package definitions defines the structures to use pre-made checks and custom
// checks in pre-commit-go.yml.
//
// Each of the struct in this file is to be embedded into pre-commit-go.yml.
// Use the comments here as a guidance to set the relevant values.
//
// The config has two root keys, 'version' and 'modes'. The valid values for
// 'modes' are 'pre-commit', 'pre-push', 'continuous-integration' and 'lint'.
// Each mode has two values; checks and max_duration. 'checks' is a list of
// check defined in this mode, 'max_duration' is the maximum duration allowed
// to run all the checks. If runtime exceeds max_duration, the run is marked as
// failed because it is too slow.
//
// Here's a sample pre-commit-go.yml file:
//
//    min_version: "0.4"
//    modes:
//      pre-commit:
//        checks:
//        - check_type: gofmt
//        - check_type: test
//          extra_args:
//          - -short
//        max_duration: 5
//      pre-push:
//        checks:
//        - check_type: build
//          extra_args: []
//      continuous-integration:
//        checks:
//        - check_type: build
//          extra_args: []
//        - check_type: gofmt
//        - check_type: goimports
//        - check_type: coverage
//          minimum_coverage: 60
//        - check_type: test
//          extra_args:
//          - -v
//          - -race
//        - check_type: custom
//          display_name: my-check
//          description: runs my check
//          command:
//          - my-check
//          - -all
//          check_exit_code: true
//          prerequisites:
//          - help_command:
//            - my-check -all
//            - -help
//            expected_exit_code: 2
//            url: github.com/me/my-check
//        max_duration: 120
//      lint:
//        checks:
//        - check_type: errcheck
//          ignores: Close
//        - blacklist: []
//          check_type: golint
//        - blacklist:
//          - ' composite literal uses unkeyed fields'
//          check_type: govet
//        max_duration: 15
package definitions

import "github.com/maruel/pre-commit-go/internal"

// CheckPrerequisite describe a Go package that is needed to run a Check.
//
// It must list a command that is to be executed and the expected exit code to
// verify that the custom tool is properly installed. If the executable is not
// detected, "go get $URL" will be executed.
type CheckPrerequisite struct {
	// HelpCommand is the help command to run to detect if this prerequisite is
	// installed or not. This command should have no adverse effect and must be
	// fast to execute.
	HelpCommand []string `yaml:"help_command"`
	// ExpectedExitCode is the exit code expected when HelpCommand is executed.
	ExpectedExitCode int `yaml:"expected_exit_code"`
	// URL is the url to fetch as `go get URL`.
	URL string `yaml:"url"`
}

// IsPresent returns true if the prerequisite is present on the system.
func (c *CheckPrerequisite) IsPresent() bool {
	_, exitCode, _ := internal.Capture("", nil, c.HelpCommand...)
	return exitCode == c.ExpectedExitCode
}

// Native checks.

// Build builds everything inside the current directory via
// 'go build ./...'.
//
// This check is mostly useful for executables, that is, "package main".
// Packages containing tests are covered via check Test.
//
// Use multiple Build instances to build multiple times with different tags.
type Build struct {
	// ExtraArgs can be used to build with different tags, e.g. to
	// build -tags foo,zoo.
	ExtraArgs []string `yaml:"extra_args"`
}

// Gofmt runs gofmt in check mode with code simplification enabled.
//
// It is almost redundant with goimports except for '-s' which goimports
// doesn't implement and gofmt doesn't require any external package.
//
// Gofmt has no configuration option. -s is always used.
type Gofmt struct {
}

// Test runs all tests via go test.
//
// Use the specialized check Coverage when -cover is desired.
//
// Use multiple Test instances to test multiple times with different flags,
// like with different tags, with or without the race detector, etc.
type Test struct {
	// ExtraArgs can be used to run the test with additional arguments like -v,
	// -short, -race, etc.
	ExtraArgs []string `yaml:"extra_args"`
}

// Non-native checks; running these require installing third party packages.

// Errcheck runs errcheck on all directories containing .go files.
//
// https://github.com/kisielk/errcheck
type Errcheck struct {
	// Ignores is the flag to pass to -ignore.
	Ignores string `yaml:"ignores"`
}

// Goimports runs goimports in check mode.
//
// Goimports has no configuration option.
//
// https://golang.org/x/tools/cmd/goimports
type Goimports struct {
}

// Golint runs golint.
//
// golint triggers false positives by design so it is preferable to use it only
// in lint mode.
//
// https://github.com/golang/lint
type Golint struct {
	// Blacklist causes this check to ignore the messages generated by golint
	// that contain one of the string listed here.
	Blacklist []string `yaml:"blacklist"`
}

// Govet runs "go tool vet".
//
// govet triggers false positives by design so it is preferable to use it only
// in lint mode.
//
// https://golang.org/cmd/vet
type Govet struct {
	// Blacklist causes this check to ignore the messages generated by go tool vet
	// that contain one of the string listed here.
	Blacklist []string `yaml:"blacklist"`
}

// Coverage runs all tests with coverage.
//
// Each testable package is run with 'go test -cover' then all coverage
// information is merged together. This means that package X/Y may create code
// coverage for package X/Z.
//
// When running on https://travis-ci.org, it tries to upload code coverage
// results to https://coveralls.io.
//
// Otherwise, only a summary is printed in case code coverage is not above
// t.MinimumCoverage.
type Coverage struct {
	// MinimumCoverage is the minimum test coverage to be generated or the check
	// is considered to fail.
	MinimumCoverage float64 `yaml:"minimum_coverage"`
}

// Extensibility.

// Custom represents a user configured check running an external program.
//
// It can be used multiple times to run multiple external checks.
type Custom struct {
	// DisplayName is check's display name, required.
	DisplayName string `yaml:"display_name"`
	// Description is check's description, optional.
	Description string `yaml:"description"`
	// Command is check's command line, required.
	Command []string `yaml:"command"`
	// CheckExitCode specifies if the check is declared to fail when exit code is
	// non-zero.
	CheckExitCode bool `yaml:"check_exit_code"`
	// Prerequisites are check's prerequisite packages to install first before
	// running the check, optional.
	Prerequisites []CheckPrerequisite `yaml:"prerequisites"`
}
