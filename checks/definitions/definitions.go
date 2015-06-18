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
//    min_version: 0.4.6
//    modes:
//      continuous-integration:
//        checks:
//          build:
//          - build_all: false
//            extra_args: []
//          coverage:
//          - use_global_inference: false
//            use_coveralls: false
//            global:
//              min_coverage: 50
//              max_coverage: 100
//            per_dir_default:
//              min_coverage: 0
//              max_coverage: 0
//            per_dir: {}
//          custom:
//          - display_name: sample-pre-commit-go-custom-check
//            description: runs the check sample-pre-commit-go-custom-check on this repository
//            command:
//            - sample-pre-commit-go-custom-check
//            - check
//            check_exit_code: true
//            prerequisites:
//            - help_command:
//              - sample-pre-commit-go-custom-check
//              - -help
//              expected_exit_code: 2
//              url: "github.com/maruel/pre-commit-go/samples/sample-pre-commit-go-custom-check"
//          gofmt:
//          - {}
//          goimports:
//          - {}
//          test:
//          - extra_args:
//            - -v
//            - -race
//        max_duration: 120
//      lint:
//        checks:
//          errcheck:
//          - ignores: Close
//          golint:
//          - blacklist: []
//          govet:
//          - blacklist:
//            - ' composite literal uses unkeyed fields'
//        max_duration: 15
//      pre-commit:
//        checks:
//          build:
//          - build_all: false
//            extra_args: []
//          gofmt:
//          - {}
//          test:
//          - extra_args:
//            - -short
//        max_duration: 5
//      pre-push:
//        checks:
//          coverage:
//          - use_global_inference: false
//            use_coveralls: false
//            global:
//              min_coverage: 50
//              max_coverage: 100
//            per_dir_default:
//              min_coverage: 0
//              max_coverage: 0
//            per_dir: {}
//          goimports:
//          - {}
//          test:
//          - extra_args:
//            - -v
//            - -race
//        max_duration: 15
//    ignore_patterns:
//    - ".*"
//    - "_*"
//    - "*.pb.go"
//
// To generate the default `pre-commit-go.yml` file, use:
//
//    pre-commit-go writeconfig
//
package definitions

import (
	"os"

	"github.com/maruel/pre-commit-go/internal"
)

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
	_, exitCode, _ := internal.Capture(cwd, nil, c.HelpCommand...)
	return exitCode == c.ExpectedExitCode
}

// Native checks.

// Build builds packages without tests via 'go build'.
type Build struct {
	BuildAll  bool
	ExtraArgs []string
}

// Coverage runs all tests with coverage.
type Coverage struct {
	UseGlobalInference bool
	UseCoveralls       bool
	Global             CoverageSettings
	PerDirDefault      CoverageSettings
	PerDir             map[string]*CoverageSettings
}

// CoverageSettings specifies coverage settings.
type CoverageSettings struct {
	MinCoverage float64
	MaxCoverage float64
}

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

// Errcheck runs errcheck on packages.
type Errcheck struct {
	Ignores string
}

// Gofmt runs gofmt in check mode with code simplification enabled.
type Gofmt struct {
}

// Goimports runs goimports in check mode.
type Goimports struct {
}

// Golint runs golint.
type Golint struct {
	Blacklist []string
}

// Govet runs "go tool vet".
type Govet struct {
	Blacklist []string `yaml:"blacklist"`
}

// Test runs all tests via go test.
type Test struct {
	ExtraArgs []string `yaml:"extra_args"`
}

// Extensibility.

// Private stuff.

var cwd string

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cwd = wd
}
