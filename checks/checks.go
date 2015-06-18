// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package checks implements pre-made checks for pcg.
//
// This package defines the `pre-commit-go.yml` configuration file format and
// implements all the checks.
package checks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
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
	URL string
}

// IsPresent returns true if the prerequisite is present on the system.
func (c *CheckPrerequisite) IsPresent() bool {
	_, exitCode, _ := internal.Capture(cwd, nil, c.HelpCommand...)
	return exitCode == c.ExpectedExitCode
}

// Check describes an check to be executed on the code base.
type Check interface {
	// GetDescription returns the check description.
	GetDescription() string
	// GetName returns the check name.
	GetName() string
	// GetPrerequisites lists all the go packages to be installed before running
	// this check.
	GetPrerequisites() []CheckPrerequisite
	// Run executes the check.
	Run(change scm.Change) error
}

// Native checks.

// Build builds packages without tests via 'go build'.
type Build struct {
	BuildAll  bool     `yaml:"build_all"`
	ExtraArgs []string `yaml:"extra_args"`
}

// GetDescription implements Check.
func (b *Build) GetDescription() string {
	return "builds all packages"
}

// GetName implements Check.
func (b *Build) GetName() string {
	return "build"
}

// GetPrerequisites implements Check.
func (b *Build) GetPrerequisites() []CheckPrerequisite {
	return nil
}

// Lock implements sync.Locker.
func (b *Build) Lock() {
	buildLock.Lock()
}

// Unlock implements sync.Locker.
func (b *Build) Unlock() {
	buildLock.Unlock()
}

// Run implements Check.
func (b *Build) Run(change scm.Change) error {
	// go build accepts packages, not files.
	// Cannot build concurrently since it leaves files in the tree.
	// TODO(maruel): Build in a temporary directory to not leave junk in the tree
	// with -o. On the other hand, ./... and -o foo are incompatible. But
	// building would have to be done in an efficient way by looking at which
	// package builds what, to not result in a O(n²) algorithm.
	args := append([]string{"go", "build"}, b.ExtraArgs...)
	out, _, err := capture(change.Repo(), append(args, change.Indirect().Packages()...)...)
	if len(out) != 0 {
		return fmt.Errorf("%s failed: %s", strings.Join(args, " "), out)
	}
	if err != nil {
		return fmt.Errorf("%s failed: %s", strings.Join(args, " "), err.Error())
	}
	return nil
}

// Copyright looks for copyright headers in all files.
type Copyright struct {
	Header string
}

// GetDescription implements Check.
func (c *Copyright) GetDescription() string {
	return "enforces all .go sources have copyright"
}

// GetName implements Check.
func (c *Copyright) GetName() string {
	return "copyright"
}

// GetPrerequisites implements Check.
func (c *Copyright) GetPrerequisites() []CheckPrerequisite {
	return nil
}

// Run implements Check.
func (c *Copyright) Run(change scm.Change) error {
	var badFiles []string
	prefix := []byte(c.Header)
	// This this serially since it's I/O bound and will compete with process
	// startup of other checks.
	for _, f := range change.Changed().GoFiles() {
		if content := change.Content(f); content != nil {
			if !bytes.HasPrefix(content, prefix) {
				badFiles = append(badFiles, f)
			}
		} else {
			badFiles = append(badFiles, f)
		}
	}
	if len(badFiles) != 0 {
		return fmt.Errorf("files have invalid copyright header:\n  %s", strings.Join(badFiles, "\n  "))
	}
	return nil
}

// Gofmt runs gofmt in check mode with code simplification enabled.
type Gofmt struct {
}

// GetDescription implements Check.
func (g *Gofmt) GetDescription() string {
	return "enforces all .go sources are formatted with 'gofmt -s'"
}

// GetName implements Check.
func (g *Gofmt) GetName() string {
	return "gofmt"
}

// GetPrerequisites implements Check.
func (g *Gofmt) GetPrerequisites() []CheckPrerequisite {
	return nil
}

// Run implements Check.
func (g *Gofmt) Run(change scm.Change) error {
	// gofmt doesn't return non-zero even if some files need to be updated.
	// gofmt accepts files, not packages.
	out, _, err := capture(change.Repo(), "gofmt", "-l", "-s", ".")
	if len(out) != 0 {
		return fmt.Errorf("these files are improperly formmatted, please run: gofmt -w -s .\n%s", out)
	}
	if err != nil {
		return fmt.Errorf("gofmt -l -s . failed: %s", err)
	}
	return nil
}

// Test runs all tests via go test.
type Test struct {
	ExtraArgs []string `yaml:"extra_args"`
}

// GetDescription implements Check.
func (t *Test) GetDescription() string {
	return "runs all tests, potentially with options (race detector, different tags, etc)"
}

// GetName implements Check.
func (t *Test) GetName() string {
	return "test"
}

// GetPrerequisites implements Check.
func (t *Test) GetPrerequisites() []CheckPrerequisite {
	return nil
}

// Run implements Check.
func (t *Test) Run(change scm.Change) error {
	// go test accepts packages, not files.
	var wg sync.WaitGroup
	testPkgs := change.Indirect().TestPackages()
	errs := make(chan error, len(testPkgs))
	for _, tp := range testPkgs {
		wg.Add(1)
		go func(testPkg string) {
			defer wg.Done()
			args := append([]string{"go", "test"}, t.ExtraArgs...)
			out, exitCode, _ := capture(change.Repo(), append(args, testPkg)...)
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

// Errcheck runs errcheck on packages.
type Errcheck struct {
	Ignores string
}

// GetDescription implements Check.
func (e *Errcheck) GetDescription() string {
	return "enforces all calls returning an error are checked using tool 'errcheck'"
}

// GetName implements Check.
func (e *Errcheck) GetName() string {
	return "errcheck"
}

// GetPrerequisites implements Check.
func (e *Errcheck) GetPrerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"errcheck", "-h"}, 2, "github.com/kisielk/errcheck"},
	}
}

// Run implements Check.
func (e *Errcheck) Run(change scm.Change) error {
	// errcheck accepts packages, not files.
	args := []string{"errcheck", "-ignore", e.Ignores}
	out, _, err := capture(change.Repo(), append(args, change.Changed().Packages()...)...)
	if len(out) != 0 {
		return fmt.Errorf("%s failed:\n%s", strings.Join(args, " "), out)
	}
	if err != nil {
		return fmt.Errorf("%s failed: %s", strings.Join(args, " "), err)
	}
	return nil
}

// Goimports runs goimports in check mode.
type Goimports struct {
}

// GetDescription implements Check.
func (g *Goimports) GetDescription() string {
	return "enforces all .go sources are formatted with 'goimports'"
}

// GetName implements Check.
func (g *Goimports) GetName() string {
	return "goimports"
}

// GetPrerequisites implements Check.
func (g *Goimports) GetPrerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"goimports", "-h"}, 2, "golang.org/x/tools/cmd/goimports"},
	}
}

// Run implements Check.
func (g *Goimports) Run(change scm.Change) error {
	// goimports accepts files, not packages.
	// goimports doesn't return non-zero even if some files need to be updated.
	out, _, err := capture(change.Repo(), append([]string{"goimports", "-l"}, change.Changed().GoFiles()...)...)
	if len(out) != 0 {
		return fmt.Errorf("these files are improperly formmatted, please run: goimports -w <files>\n%s", out)
	}
	if err != nil {
		return fmt.Errorf("goimports -w . failed: %s", err)
	}
	return nil
}

// Golint runs golint.
type Golint struct {
	Blacklist []string
}

// GetDescription implements Check.
func (g *Golint) GetDescription() string {
	return "enforces all .go sources passes golint"
}

// GetName implements Check.
func (g *Golint) GetName() string {
	return "golint"
}

// GetPrerequisites implements Check.
func (g *Golint) GetPrerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"golint", "-h"}, 2, "github.com/golang/lint/golint"},
	}
}

// Run implements Check.
func (g *Golint) Run(change scm.Change) error {
	// - accepts packages, not files.
	// - doesn't return non-zero ever.
	// - doesn't like multiple packages per call.
	// - "." is not recursive.
	pkgs := change.Changed().Packages()
	resultsC := make(chan []string, len(pkgs))
	for _, pkg := range pkgs {
		go func(p string) {
			r := []string{}
			out, _, _ := capture(change.Repo(), "golint", p)
			for _, line := range strings.Split(string(out), "\n") {
				if len(line) == 0 {
					continue
				}
				for _, b := range g.Blacklist {
					if strings.Contains(line, b) {
						goto skip
					}
				}
				r = append(r, line)
			skip:
			}
			resultsC <- r
		}(pkg)
	}

	results := []string{}
	for i := 0; i < len(pkgs); i++ {
		results = append(results, <-resultsC...)
	}
	if len(results) != 0 {
		return errors.New(strings.Join(results, "\n"))
	}
	return nil
}

// Govet runs "go tool vet".
type Govet struct {
	Blacklist []string
}

// GetDescription implements Check.
func (g *Govet) GetDescription() string {
	return "enforces all .go sources passes go tool vet"
}

// GetName implements Check.
func (g *Govet) GetName() string {
	return "govet"
}

// GetPrerequisites implements Check.
func (g *Govet) GetPrerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"go", "tool", "vet", "-h"}, 1, "golang.org/x/tools/cmd/vet"},
	}
}

// Run implements Check.
func (g *Govet) Run(change scm.Change) error {
	// - accepts packages, not files.
	// - returns non-zero on report.
	// - accepts multiple packages per call.
	// - "." is recursive.
	// Ignore the return code since we ignore many errors.
	out, _, _ := capture(change.Repo(), "go", "tool", "vet", "-all", ".")
	result := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) == 0 {
			continue
		}
		for _, b := range g.Blacklist {
			if strings.Contains(line, b) {
				continue
			}
		}
		// TODO(maruel): Filter on change.Changed().GoFiles().
		result = append(result, line)
	}
	if len(result) != 0 {
		return errors.New(strings.Join(result, "\n"))
	}
	return nil
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

// GetDescription implements Check.
func (c *Custom) GetDescription() string {
	if c.Description != "" {
		return c.Description
	}
	return "runs a custom check from an external package"
}

// GetName implements Check.
func (c *Custom) GetName() string {
	return "custom"
}

// GetPrerequisites implements Check.
func (c *Custom) GetPrerequisites() []CheckPrerequisite {
	return c.Prerequisites
}

// Run implements Check.
func (c *Custom) Run(change scm.Change) error {
	// TODO(maruel): Make what is passed to the command configurable, e.g. one of:
	// (Changed, Indirect, All) x (GoFiles, Packages, TestPackages)
	out, exitCode, err := capture(change.Repo(), c.Command...)
	if exitCode != 0 && c.CheckExitCode {
		return fmt.Errorf("\"%s\" failed with code %d:\n%s", strings.Join(c.Command, " "), exitCode, out)
	}
	return err
}

// Rest.

// KnownChecks is the map of all known checks per check name.
var KnownChecks = map[string]func() Check{
	(&Build{}).GetName():     func() Check { return &Build{} },
	(&Copyright{}).GetName(): func() Check { return &Copyright{} },
	(&Coverage{}).GetName():  func() Check { return &Coverage{} },
	(&Custom{}).GetName():    func() Check { return &Custom{} },
	(&Errcheck{}).GetName():  func() Check { return &Errcheck{} },
	(&Gofmt{}).GetName():     func() Check { return &Gofmt{} },
	(&Goimports{}).GetName(): func() Check { return &Goimports{} },
	(&Golint{}).GetName():    func() Check { return &Golint{} },
	(&Govet{}).GetName():     func() Check { return &Govet{} },
	(&Test{}).GetName():      func() Check { return &Test{} },
}

// Private stuff.

// See build.Run() for information.
var buildLock sync.Mutex

// cwd provides a valid path to CheckPrerequisite.IsPresent().
var cwd string

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cwd = wd
}
