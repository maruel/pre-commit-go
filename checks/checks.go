// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package checks implements pre-made checks for pre-commit-go.
//
// This package defines the `pre-commit-go.yml` configuration file format and
// implements all the checks. For conciseness, all the check configuration
// struct are defined in the package `definitions` below.
package checks

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/maruel/pre-commit-go/checks/definitions"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
)

// Checks are alias of the corresponding checks in package definitions. The
// reason is so the definitions package documentation at
// https://godoc.org/github.com/maruel/pre-commit-go/checks/definitions is
// clean and the code in this package focus on the implementation.
//
// They are not exported since they are meant to be created by deserializing a
// pre-commit-go.yml, not to be instantiated manually.

// See build.Run() for information.
var buildLock sync.Mutex

// Check describes an check to be executed on the code base.
type Check interface {
	// GetDescription returns the check description.
	GetDescription() string
	// GetName returns the check name.
	GetName() string
	// GetPrerequisites lists all the go packages to be installed before running
	// this check.
	GetPrerequisites() []definitions.CheckPrerequisite
	// Run executes the check.
	Run(change scm.Change) error
}

// Native checks.

type build definitions.Build

func (b *build) GetDescription() string {
	return "builds all packages"
}

func (b *build) GetName() string {
	return "build"
}

func (b *build) GetPrerequisites() []definitions.CheckPrerequisite {
	return nil
}

func (b *build) Lock() {
	buildLock.Lock()
}

func (b *build) Unlock() {
	buildLock.Unlock()
}

func (b *build) Run(change scm.Change) error {
	// go build accepts packages, not files.
	// Cannot build concurrently since it leaves files in the tree.
	// TODO(maruel): Build in a temporary directory to not leave junk in the tree
	// with -o. On the other hand, ./... and -o foo are incompatible. But
	// building would have to be done in an efficient way by looking at which
	// package builds what, to not result in a O(n²) algorithm.
	args := append([]string{"go", "build"}, b.ExtraArgs...)
	out, _, err := internal.Capture("", nil, append(args, change.Indirect().Packages()...)...)
	if len(out) != 0 {
		return fmt.Errorf("%s failed: %s", strings.Join(args, " "), out)
	}
	if err != nil {
		return fmt.Errorf("%s failed: %s", strings.Join(args, " "), err.Error())
	}
	return nil
}

type gofmt definitions.Gofmt

func (g *gofmt) GetDescription() string {
	return "enforces all .go sources are formatted with 'gofmt -s'"
}

func (g *gofmt) GetName() string {
	return "gofmt"
}

func (g *gofmt) GetPrerequisites() []definitions.CheckPrerequisite {
	return nil
}

func (g *gofmt) Run(change scm.Change) error {
	// gofmt doesn't return non-zero even if some files need to be updated.
	// gofmt accepts files, not packages.
	out, _, err := internal.Capture("", nil, "gofmt", "-l", "-s", ".")
	if len(out) != 0 {
		return fmt.Errorf("these files are improperly formmatted, please run: gofmt -w -s .\n%s", out)
	}
	if err != nil {
		return fmt.Errorf("gofmt -l -s . failed: %s", err)
	}
	return nil
}

type test definitions.Test

func (t *test) GetDescription() string {
	return "runs all tests, potentially with options (race detector, different tags, etc)"
}

func (t *test) GetName() string {
	return "test"
}

func (t *test) GetPrerequisites() []definitions.CheckPrerequisite {
	return nil
}

func (t *test) Run(change scm.Change) error {
	// go test accepts packages, not files.
	var wg sync.WaitGroup
	testPkgs := change.Indirect().TestPackages()
	errs := make(chan error, len(testPkgs))
	for _, tp := range testPkgs {
		wg.Add(1)
		go func(testPkg string) {
			defer wg.Done()
			args := append([]string{"go", "test"}, t.ExtraArgs...)
			out, exitCode, _ := internal.Capture("", nil, append(args, testPkg)...)
			if exitCode != 0 {
				errs <- fmt.Errorf("%s failed:\n%s", strings.Join(args, " "), out)
			}
		}(tp)
	}
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
	}
	return nil
}

type errcheck definitions.Errcheck

func (e *errcheck) GetDescription() string {
	return "enforces all calls returning an error are checked using tool 'errcheck'"
}

func (e *errcheck) GetName() string {
	return "errcheck"
}

func (e *errcheck) GetPrerequisites() []definitions.CheckPrerequisite {
	return []definitions.CheckPrerequisite{
		{[]string{"errcheck", "-h"}, 2, "github.com/kisielk/errcheck"},
	}
}

func (e *errcheck) Run(change scm.Change) error {
	// errcheck accepts packages, not files.
	args := []string{"errcheck", "-ignore", e.Ignores}
	out, _, err := internal.Capture("", nil, append(args, change.Changed().Packages()...)...)
	if len(out) != 0 {
		return fmt.Errorf("%s failed:\n%s", strings.Join(args, " "), out)
	}
	if err != nil {
		return fmt.Errorf("%s failed: %s", strings.Join(args, " "), err)
	}
	return nil
}

type goimports definitions.Goimports

func (g *goimports) GetDescription() string {
	return "enforces all .go sources are formatted with 'goimports'"
}

func (g *goimports) GetName() string {
	return "goimports"
}

func (g *goimports) GetPrerequisites() []definitions.CheckPrerequisite {
	return []definitions.CheckPrerequisite{
		{[]string{"goimports", "-h"}, 2, "golang.org/x/tools/cmd/goimports"},
	}
}

func (g *goimports) Run(change scm.Change) error {
	// goimports accepts files, not packages.
	// goimports doesn't return non-zero even if some files need to be updated.
	out, _, err := internal.Capture("", nil, append([]string{"goimports", "-l"}, change.Changed().GoFiles()...)...)
	if len(out) != 0 {
		return fmt.Errorf("these files are improperly formmatted, please run: goimports -w <files>\n%s", out)
	}
	if err != nil {
		return fmt.Errorf("goimports -w . failed: %s", err)
	}
	return nil
}

type golint definitions.Golint

func (g *golint) GetDescription() string {
	return "enforces all .go sources passes golint"
}

func (g *golint) GetName() string {
	return "golint"
}

func (g *golint) GetPrerequisites() []definitions.CheckPrerequisite {
	return []definitions.CheckPrerequisite{
		{[]string{"golint", "-h"}, 2, "github.com/golang/lint/golint"},
	}
}

func (g *golint) Run(change scm.Change) error {
	// golint accepts packages, not files.
	// golint doesn't return non-zero ever.
	out, _, _ := internal.Capture("", nil, "golint", "./...")
	result := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		for _, b := range g.Blacklist {
			if strings.Contains(line, b) {
				continue
			}
		}
		result = append(result, line)
	}
	if len(result) == 0 {
		return errors.New(strings.Join(result, "\n"))
	}
	return nil
}

type govet definitions.Govet

func (g *govet) GetDescription() string {
	return "enforces all .go sources passes go tool vet"
}

func (g *govet) GetName() string {
	return "govet"
}

func (g *govet) GetPrerequisites() []definitions.CheckPrerequisite {
	return []definitions.CheckPrerequisite{
		{[]string{"go", "tool", "vet", "-h"}, 1, "golang.org/x/tools/cmd/vet"},
	}
}

func (g *govet) Run(change scm.Change) error {
	// govet accepts files, not packages.
	// Ignore the return code since we ignore many errors.
	out, _, _ := internal.Capture("", nil, "go", "tool", "vet", "-all", ".")
	result := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		for _, b := range g.Blacklist {
			if strings.Contains(line, b) {
				continue
			}
		}
		result = append(result, line)
	}
	if len(result) == 0 {
		return errors.New(strings.Join(result, "\n"))
	}
	return nil
}

// Extensibility.

type custom definitions.Custom

func (c *custom) GetDescription() string {
	if c.Description != "" {
		return c.Description
	}
	return "runs a custom check from an external package"
}

func (c *custom) GetName() string {
	return "custom"
}

func (c *custom) GetPrerequisites() []definitions.CheckPrerequisite {
	return c.Prerequisites
}

func (c *custom) Run(change scm.Change) error {
	// TODO(maruel): Make what is passed to the command configurable, e.g. one of:
	// (Changed, Indirect, All) x (GoFiles, Packages, TestPackages)
	out, exitCode, err := internal.Capture("", nil, c.Command...)
	if exitCode != 0 && c.CheckExitCode {
		return fmt.Errorf("\"%s\" failed with code %d:\n%s", strings.Join(c.Command, " "), exitCode, out)
	}
	return err
}

// Rest.

// KnownChecks is the map of all known checks per check name.
var KnownChecks = map[string]func() Check{
	(&build{}).GetName():     func() Check { return &build{} },
	(&Coverage{}).GetName():  func() Check { return &Coverage{} },
	(&custom{}).GetName():    func() Check { return &custom{} },
	(&errcheck{}).GetName():  func() Check { return &errcheck{} },
	(&gofmt{}).GetName():     func() Check { return &gofmt{} },
	(&goimports{}).GetName(): func() Check { return &goimports{} },
	(&golint{}).GetName():    func() Check { return &golint{} },
	(&govet{}).GetName():     func() Check { return &govet{} },
	(&test{}).GetName():      func() Check { return &test{} },
}
