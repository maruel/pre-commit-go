// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package checks implements pre-made checks for pre-commit-go.
package checks

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
	return "builds all packages that do not contain tests, usually all directories with package 'main'"
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
	// Cannot build concurrently since it leaves files in the tree.
	// TODO(maruel): Build in a temporary directory to not leave junk in the tree
	// with -o. On the other hand, ./... and -o foo are incompatible. But
	// building would have to be done in an efficient way by looking at which
	// package builds what, to not result in a O(n²) algorithm.
	args := []string{"go", "build"}
	args = append(args, b.ExtraArgs...)
	args = append(args, "./...")
	out, _, err := internal.Capture("", nil, args...)
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
	// Add tests manually instead of using './...'. The reason is that it permits
	// running all the tests concurrently, which saves a lot of time when there's
	// many packages.
	var wg sync.WaitGroup
	tds := change.TestDirs()
	errs := make(chan error, len(tds))
	for _, td := range tds {
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
			args = append(args, rel)
			out, exitCode, _ := internal.Capture("", nil, args...)
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
	dirs := change.SourceDirs()
	args := make([]string, 0, len(dirs)+2)
	args = append(args, "errcheck", "-ignore", e.Ignores)
	for _, d := range dirs {
		rel, err := relToGOPATH(d)
		if err != nil {
			return err
		}
		args = append(args, rel)
	}
	out, _, err := internal.Capture("", nil, args...)
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
	// goimports doesn't return non-zero even if some files need to be updated.
	out, _, err := internal.Capture("", nil, "goimports", "-l", ".")
	if len(out) != 0 {
		return fmt.Errorf("these files are improperly formmatted, please run: goimports -w .\n%s", out)
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

type coverage definitions.Coverage

func (c *coverage) GetDescription() string {
	return "enforces minimum test coverage on all packages that are not 'main'"
}

func (c *coverage) GetName() string {
	return "coverage"
}

func (c *coverage) GetPrerequisites() []definitions.CheckPrerequisite {
	toInstall := []definitions.CheckPrerequisite{
		{[]string{"go", "tool", "cover", "-h"}, 1, "golang.org/x/tools/cmd/cover"},
	}
	if IsContinuousIntegration() {
		toInstall = append(toInstall, definitions.CheckPrerequisite{[]string{"goveralls", "-h"}, 2, "github.com/mattn/goveralls"})
	}
	return toInstall
}

func (c *coverage) Run(change scm.Change) (err error) {
	// TODO(maruel): Kept because we may have to revert to using broader
	// instrumentation due to OS command line argument length limit.
	//pkgRoot, _ := os.Getwd()
	//pkg, err2 := relToGOPATH(pkgRoot)
	//if err2 != nil {
	//	return err2
	//}
	pds := change.PackageDirs()
	coverPkg := ""
	for i, p := range pds {
		if i != 0 {
			coverPkg += ","
		}
		rel, err2 := relToGOPATH(p)
		if err2 != nil {
			return err2
		}
		coverPkg += rel
	}

	tds := change.TestDirs()
	if len(tds) == 0 {
		return nil
	}

	tmpDir, err2 := ioutil.TempDir("", "pre-commit-go")
	if err2 != nil {
		return err2
	}
	defer func() {
		err2 := internal.RemoveAll(tmpDir)
		if err == nil {
			err = err2
		}
	}()

	// This part is similar to Test.Run() except that it passes a unique
	// -coverprofile file name, so that all the files can later be merged into a
	// single file.
	var wg sync.WaitGroup
	errs := make(chan error, len(tds))

	for i, td := range tds {
		wg.Add(1)
		go func(index int, testDir string) {
			defer wg.Done()
			// TODO(maruel): Maybe fallback to 'pkg + "/..."' and post process to
			// remove uninteresting directories. The rationale is that it will
			// eventually blow up the OS specific command argument length.
			args := []string{
				"go", "test", "-v", "-covermode=count", "-coverpkg", coverPkg,
				"-coverprofile", filepath.Join(tmpDir, fmt.Sprintf("test%d.cov", index)),
			}
			out, exitCode, _ := internal.Capture(testDir, nil, args...)
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
	out, _, err2 := internal.Capture("", nil, "go", "tool", "cover", "-func", profilePath)
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
	if total < c.MinimumCoverage {
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
	if IsContinuousIntegration() {
		// Make sure to have registered to https://coveralls.io first!
		out, _, err3 := internal.Capture("", nil, "goveralls", "-coverprofile", profilePath)
		fmt.Printf("%s", out)
		if err2 == nil {
			err2 = err3
		}
	}
	return err2
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
	out, exitCode, err := internal.Capture("", nil, c.Command...)
	if exitCode != 0 && c.CheckExitCode {
		return fmt.Errorf("\"%s\" failed:\n%s", strings.Join(c.Command, " "), out)
	}
	return err
}

// KnownChecks is the map of all known checks per check name.
var KnownChecks map[string]Check

func init() {
	known := []Check{
		&build{},
		&gofmt{},
		&test{},
		&errcheck{},
		&goimports{},
		&golint{},
		&govet{},
		&coverage{},
		&custom{},
	}
	KnownChecks = map[string]Check{}
	for _, k := range known {
		name := k.GetName()
		if _, ok := KnownChecks[name]; ok {
			panic(fmt.Sprintf("duplicate check named %s", name))
		}
		KnownChecks[name] = k
	}
}
