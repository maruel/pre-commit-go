// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// CheckPrerequisite describe a Go package that is needed to run a Check. It
// must list a command that is to be executed and the expected exit code. This
// permits to ensure the command is properly installed. If not, "go get $URL"
// will be executed.
type CheckPrerequisite struct {
	HelpCommand      []string
	ExpectedExitCode int
	URL              string
}

// Check describes an check to be executed on the code base.
type Check interface {
	enabled() bool
	maxDuration() float64
	name() string
	run() error
	prerequisites() []CheckPrerequisite
}

// CheckCommon defines the common properties of a check to be serialized in the
// configuration file.
type CheckCommon struct {
	Enabled     bool    `json:"enabled"`
	MaxDuration float64 `json:"maxduration"` // In seconds. Default to MaxDuration at global scope.
}

func (c *CheckCommon) enabled() bool {
	return c.Enabled
}

func (c *CheckCommon) maxDuration() float64 {
	return c.MaxDuration
}

// Native checks.

// Build builds everything inside the current directory via 'go build ./...'.
type Build struct {
	CheckCommon
	Tags []string `json:"tags"`
}

func (b *Build) name() string {
	return "build"
}

func (b *Build) prerequisites() []CheckPrerequisite {
	return nil
}

func (b *Build) run() error {
	tags := b.Tags
	if len(tags) == 0 {
		tags = []string{""}
	}
	for _, tag := range tags {
		args := []string{"go", "build"}
		if len(tag) != 0 {
			args = append(args, "-tags", tag)
		}
		args = append(args, "./...")
		out, _, err := capture(args...)
		if len(out) != 0 {
			return fmt.Errorf("%s failed: %s", strings.Join(args, " "), out)
		}
		if err != nil {
			return fmt.Errorf("%s failed: %s", strings.Join(args, " "), err.Error())
		}
	}
	return nil
}

// Gofmt runs gofmt in check mode with code simplification enabled.
// It is *almost* redundant with goimports except for '-s' which goimports
// doesn't implement.
type Gofmt struct {
	CheckCommon
}

func (g *Gofmt) name() string {
	return "gofmt"
}

func (g *Gofmt) prerequisites() []CheckPrerequisite {
	return nil
}

func (g *Gofmt) run() error {
	// gofmt doesn't return non-zero even if some files need to be updated.
	out, _, err := capture("gofmt", "-l", "-s", ".")
	if len(out) != 0 {
		return fmt.Errorf("these files are improperly formmatted, please run: gofmt -w -s .\n%s", out)
	}
	if err != nil {
		return fmt.Errorf("gofmt -l -s . failed: %s", err)
	}
	return nil
}

// Test runs all tests.
type Test struct {
	CheckCommon
	ExtraArgs []string `json:"extraargs"` // Additional arguments to pass, like -race.
	Tags      []string `json:"tags"`      // Used to run the tests N times with N tags set.
}

func (t *Test) name() string {
	return "test"
}

func (t *Test) prerequisites() []CheckPrerequisite {
	return nil
}

func (t *Test) run() error {
	// Add tests manually instead of using './...'. The reason is that it permits
	// running all the tests concurrently, which saves a lot of time when there's
	// many packages.
	var wg sync.WaitGroup
	testDirs := goDirs(true)
	tags := t.Tags
	if len(tags) == 0 {
		tags = []string{""}
	}
	for _, tag := range tags {
		errs := make(chan error, len(testDirs))
		for _, td := range testDirs {
			wg.Add(1)
			go func(testDir string) {
				defer wg.Done()
				rel, err := relToGOPATH(testDir)
				if err != nil {
					errs <- err
					return
				}
				args := []string{"go", "test"}
				args = append(args, t.ExtraArgs...)
				if len(tag) != 0 {
					args = append(args, "-tags", tag)
				}
				args = append(args, rel)
				out, exitCode, _ := capture(args...)
				if exitCode != 0 {
					errs <- fmt.Errorf("%s failed:\n%s", strings.Join(args, " "), out)
				}
			}(td)
		}
		wg.Wait()
		select {
		case err := <-errs:
			return err
		default:
		}
	}
	return nil
}

// Non-native checks.

// Errcheck runs errcheck on all directories containing .go files.
type Errcheck struct {
	CheckCommon
	Ignores string `json:"ignores"`
}

func (e *Errcheck) name() string {
	return "errcheck"
}

func (e *Errcheck) prerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"errcheck", "-h"}, 2, "github.com/kisielk/errcheck"},
	}
}

func (e *Errcheck) run() error {
	dirs := goDirs(false)
	args := make([]string, 0, len(dirs)+2)
	args = append(args, "errcheck", "-ignore", e.Ignores)
	for _, d := range dirs {
		rel, err := relToGOPATH(d)
		if err != nil {
			return err
		}
		args = append(args, rel)
	}
	out, _, err := capture(args...)
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
	CheckCommon
}

func (g *Goimports) name() string {
	return "goimports"
}

func (g *Goimports) prerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"goimports", "-h"}, 2, "golang.org/x/tools/cmd/goimports"},
	}
}

func (g *Goimports) run() error {
	// goimports doesn't return non-zero even if some files need to be updated.
	out, _, err := capture("goimports", "-l", ".")
	if len(out) != 0 {
		return fmt.Errorf("these files are improperly formmatted, please run: goimports -w .\n%s", out)
	}
	if err != nil {
		return fmt.Errorf("goimports -w . failed: %s", err)
	}
	return nil
}

// Golint runs golint.
// There starts the cheezy part that may return false positives. I'm sorry
// David.
type Golint struct {
	CheckCommon
	Blacklist []string `json:"blacklist"`
}

func (g *Golint) name() string {
	return "golint"
}

func (g *Golint) prerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"golint", "-h"}, 2, "github.com/golang/lint/golint"},
	}
}

func (g *Golint) run() error {
	// golint doesn't return non-zero ever.
	out, _, _ := capture("golint", "./...")
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

// Govet runs "go tool vet".
type Govet struct {
	CheckCommon
	Blacklist []string `json:"blacklist"`
}

func (g *Govet) name() string {
	return "govet"
}

func (g *Govet) prerequisites() []CheckPrerequisite {
	return []CheckPrerequisite{
		{[]string{"go", "tool", "vet", "-h"}, 1, "golang.org/x/tools/cmd/vet"},
	}
}

func (g *Govet) run() error {
	// Ignore the return code since we ignore many errors.
	out, _, _ := capture("go", "tool", "vet", "-all", ".")
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

// TestCoverage runs all tests with coverage.
type TestCoverage struct {
	CheckCommon
	Minimum float64 `json:"minimum"`
}

func (t *TestCoverage) name() string {
	return "testcoverage"
}

func (t *TestCoverage) prerequisites() []CheckPrerequisite {
	toInstall := []CheckPrerequisite{
		{[]string{"go", "tool", "cover", "-h"}, 1, "golang.org/x/tools/cmd/cover"},
	}
	if len(os.Getenv("TRAVIS_JOB_ID")) != 0 {
		toInstall = append(toInstall, CheckPrerequisite{[]string{"goveralls", "-h"}, 2, "github.com/mattn/goveralls"})
	}
	return toInstall
}

func (t *TestCoverage) run() (err error) {
	pkgRoot, _ := os.Getwd()
	pkg, err2 := relToGOPATH(pkgRoot)
	if err2 != nil {
		return err2
	}
	testDirs := goDirs(true)
	if len(testDirs) == 0 {
		return nil
	}

	tmpDir, err2 := ioutil.TempDir("", "pre-commit-go")
	if err2 != nil {
		return err2
	}
	defer func() {
		err2 := os.RemoveAll(tmpDir)
		if err == nil {
			err = err2
		}
	}()

	// This part is similar to Test.run() except that it passes a unique
	// -coverprofile file name, so that all the files can later be merged into a
	// single file.
	var wg sync.WaitGroup
	errs := make(chan error, len(testDirs))
	for i, td := range testDirs {
		wg.Add(1)
		go func(index int, testDir string) {
			defer wg.Done()
			args := []string{
				"go", "test", "-v", "-covermode=count", "-coverpkg", pkg + "/...",
				"-coverprofile", filepath.Join(tmpDir, fmt.Sprintf("test%d.cov", index)),
			}
			out, exitCode, _ := captureWd(testDir, args...)
			if exitCode != 0 {
				errs <- fmt.Errorf("%s %s failed:\n%s", strings.Join(args, " "), testDir, out)
			}
		}(i, td)
	}
	wg.Wait()

	// Merge the profiles. Sums all the counts.
	// Format is "file.go:XX.YY,ZZ.II J K"
	// J is number of statements, K is count.
	files, err2 := filepath.Glob(filepath.Join(tmpDir, "test*.cov"))
	if err2 != nil {
		return err2
	}
	if len(files) == 0 {
		select {
		case err2 := <-errs:
			return err2
		default:
			return errors.New("no coverage found")
		}
	}
	counts := map[string]int{}
	for _, file := range files {
		f, err2 := os.Open(file)
		if err2 != nil {
			return err2
		}
		s := bufio.NewScanner(f)
		// Strip the first line.
		s.Scan()
		count := 0
		for s.Scan() {
			items := rsplitn(s.Text(), " ", 2)
			count, err2 = strconv.Atoi(items[1])
			if err2 != nil {
				break
			}
			counts[items[0]] += int(count)
		}
		f.Close()
		if err2 != nil {
			return err2
		}
	}
	profilePath := filepath.Join(tmpDir, "profile.cov")
	f, err2 := os.Create(profilePath)
	if err2 != nil {
		return err2
	}
	stms := make([]string, 0, len(counts))
	for k := range counts {
		stms = append(stms, k)
	}
	sort.Strings(stms)
	_, _ = io.WriteString(f, "mode: count\n")
	for _, stm := range stms {
		fmt.Fprintf(f, "%s %d\n", stm, counts[stm])
	}
	f.Close()

	// Analyze the results.
	out, _, err2 := capture("go", "tool", "cover", "-func", profilePath)
	type fn struct {
		loc  string
		name string
	}
	coverage := map[fn]float64{}
	var total float64
	for i, line := range strings.Split(out, "\n") {
		if i == 0 || len(line) == 0 {
			// First or last line.
			continue
		}
		items := strings.SplitN(line, "\t", 2)
		loc := items[0]
		if len(items) == 1 {
			panic(fmt.Sprintf("%#v %#v", line, items))
		}
		items = strings.SplitN(strings.TrimLeft(items[1], "\t"), "\t", 2)
		name := items[0]
		percentStr := strings.TrimLeft(items[1], "\t")
		percent, err2 := strconv.ParseFloat(percentStr[:len(percentStr)-1], 64)
		if err2 != nil {
			return fmt.Errorf("malformed coverage file")
		}
		if loc == "total:" {
			total = percent
		} else {
			coverage[fn{loc, name}] = percent
		}
	}
	if total < t.Minimum {
		partial := 0
		for _, percent := range coverage {
			if percent < 100. {
				partial++
			}
		}
		err2 = fmt.Errorf("code coverage: %3.1f%%; %d untested functions", total, partial)
	}
	if err2 == nil {
		select {
		case err2 = <-errs:
		default:
		}
	}

	// Sends to coveralls.io if applicable.
	if len(os.Getenv("TRAVIS_JOB_ID")) != 0 {
		// Make sure to have registered to https://coveralls.io first!
		out, _, err3 := capture("goveralls", "-coverprofile", profilePath)
		fmt.Printf("%s", out)
		if err2 == nil {
			err2 = err3
		}
	}
	return err2
}

// CustomCheck represents a user configured check.
type CustomCheck struct {
	CheckCommon
	Name          string              `json:"name"`          // Check display name.
	Command       []string            `json:"command"`       // Check command line.
	CheckExitCode bool                `json:"checkexitcode"` // Check fails if exit code is non-zero.
	Prerequisites []CheckPrerequisite `json:"prerequisites"`
}

func (c *CustomCheck) name() string {
	return c.Name
}

func (c *CustomCheck) prerequisites() []CheckPrerequisite {
	return c.Prerequisites
}

func (c *CustomCheck) run() error {
	out, exitCode, err := capture(c.Command...)
	if exitCode != 0 && c.CheckExitCode {
		return fmt.Errorf("%d failed:\n%s", strings.Join(c.Command, " "), out)
	}
	return err
}
