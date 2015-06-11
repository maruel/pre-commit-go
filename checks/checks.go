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
	"log"
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

type coverage definitions.Coverage

func (c *coverage) GetDescription() string {
	return "enforces minimum test coverage on all packages"
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
	// go test accepts packages, not files.
	coverPkg := ""
	// TODO(maruel): Make skipping packages without tests configurable.
	// TODO(maruel): Make skipping arbitrary packages configurable.
	for i, p := range change.All().Packages() {
		if i != 0 {
			coverPkg += ","
		}
		coverPkg += p
	}

	testPkgs := change.All().TestPackages()
	if len(testPkgs) == 0 {
		return nil
	}

	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	if err != nil {
		return err
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
	errs := make(chan error, len(testPkgs))
	for i, tp := range testPkgs {
		wg.Add(1)
		go func(index int, testPkg string) {
			defer wg.Done()
			// TODO(maruel): Maybe fallback to 'pkg + "/..."' and post process to
			// remove uninteresting directories. The rationale is that it will
			// eventually blow up the OS specific command argument length.
			args := []string{
				"go", "test", "-v", "-covermode=count", "-coverpkg", coverPkg,
				"-coverprofile", filepath.Join(tmpDir, fmt.Sprintf("test%d.cov", index)),
				testPkg,
			}
			out, exitCode, _ := internal.Capture("", nil, args...)
			if exitCode != 0 {
				errs <- fmt.Errorf("%s %s failed:\n%s", strings.Join(args, " "), testPkg, out)
			}
		}(i, tp)
	}
	wg.Wait()

	// Merge the profiles. Sums all the counts.
	// Format is "file.go:XX.YY,ZZ.II J K"
	// J is number of statements, K is count.
	files, err := filepath.Glob(filepath.Join(tmpDir, "test*.cov"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		select {
		case err = <-errs:
			return err
		default:
			return errors.New("no coverage found")
		}
	}
	profilePath := filepath.Join(tmpDir, "profile.cov")
	f, err := os.Create(profilePath)
	if err != nil {
		return err
	}
	if err = mergeCoverage(files, f); err != nil {
		f.Close()
		return err
	}
	f.Close()

	coverage, total, partial, err := loadProfile(profilePath)
	if err != nil {
		return err
	}
	log.Printf("%d functions profiled in %s", len(coverage), coverPkg)

	// TODO(maruel): Calculate the sorted list only when -verbose is specified.
	maxLoc := 0
	maxName := 0
	sort.Sort(coverage)
	for _, item := range coverage {
		if item.percent < 100. {
			if l := len(item.loc); l > maxLoc {
				maxLoc = l
			}
			if l := len(item.name); l > maxName {
				maxName = l
			}
		}
	}
	for _, item := range coverage {
		if item.percent < 100. {
			log.Printf("%-*s %-*s %1.1f%%", maxLoc, item.loc, maxName, item.name, item.percent)
		}
	}
	if total < c.MinimumCoverage {
		err = fmt.Errorf("code coverage: %3.1f%% < %d%%; %d untested functions", total, c.MinimumCoverage, partial)
	} else {
		log.Printf("code coverage: %3.1f%% >= %d%%; %d untested functions", total, c.MinimumCoverage, partial)
	}
	if err == nil {
		select {
		case err = <-errs:
		default:
		}
	}

	// Sends to coveralls.io if applicable.
	if IsContinuousIntegration() {
		// TODO(maruel): Test with all of drone.io, travis-ci.org, etc. In theory
		// goveralls tries to be smart but we need to ensure it works for all
		// services. Please send a pull request if it doesn't work for you.
		out, _, err2 := internal.Capture("", nil, "goveralls", "-coverprofile", profilePath)
		// Don't fail the build.
		if err2 != nil {
			fmt.Printf("%s", out)
		}
	}
	return
}

// mergeCoverage merges multiple coverage profiles into out.
//
// It sums all the counts of each profile.
//
// Format is "file.go:XX.YY,ZZ.II J K"
// J is number of statements, K is count.
func mergeCoverage(files []string, out io.Writer) error {
	counts := map[string]int{}
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		s := bufio.NewScanner(f)
		// Strip the first line.
		s.Scan()
		count := 0
		for s.Scan() {
			items := rsplitn(s.Text(), " ", 2)
			count, err = strconv.Atoi(items[1])
			if err != nil {
				break
			}
			counts[items[0]] += int(count)
		}
		f.Close()
		if err != nil {
			return err
		}
	}
	stms := make([]string, 0, len(counts))
	for k := range counts {
		stms = append(stms, k)
	}
	sort.Strings(stms)
	if _, err := io.WriteString(out, "mode: count\n"); err != nil {
		return err
	}
	for _, stm := range stms {
		if _, err := fmt.Fprintf(out, "%s %d\n", stm, counts[stm]); err != nil {
			return err
		}
	}
	return nil
}

// loadProfile analyzes the results of a coverage profile.
func loadProfile(p string) (ordered, float64, int, error) {
	out, code, err := internal.Capture("", nil, "go", "tool", "cover", "-func", p)
	if code != 0 || err != nil {
		return nil, 0, 0, fmt.Errorf("go tool cover failed with code %d; %s", code, err)
	}
	coverage := ordered{}
	partial := 0
	var total float64
	for i, line := range strings.Split(out, "\n") {
		if i == 0 || len(line) == 0 {
			// First or last line.
			continue
		}
		items := strings.SplitN(line, "\t", 2)
		loc := items[0]
		if strings.HasSuffix(loc, ":") {
			loc = loc[:len(loc)-1]
		}
		if len(items) == 1 {
			panic(fmt.Sprintf("%#v %#v", line, items))
		}
		items = strings.SplitN(strings.TrimLeft(items[1], "\t"), "\t", 2)
		name := items[0]
		percentStr := strings.TrimLeft(items[1], "\t")
		percent, err := strconv.ParseFloat(percentStr[:len(percentStr)-1], 64)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("malformed coverage file")
		}
		if loc == "total" {
			total = percent
		} else {
			coverage = append(coverage, covered{percent, loc, name})
			if percent < 100. {
				partial++
			}
		}
	}
	return coverage, total, partial, nil
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

type ordered []covered

func (o ordered) Len() int      { return len(o) }
func (o ordered) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o ordered) Less(i, j int) bool {
	if o[i].percent > o[j].percent {
		return true
	}
	if o[i].percent < o[j].percent {
		return false
	}
	if o[i].loc < o[j].loc {
		return true
	}
	return false
}

type covered struct {
	percent float64
	loc     string
	name    string
}
