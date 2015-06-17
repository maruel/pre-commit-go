// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// coverage is a large check so it is in its own file.
//
// It is designed to be usable standalone.

package checks

import (
	"bufio"
	"bytes"
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
	"github.com/maruel/pre-commit-go/checks/internal/cover"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
)

// Coverage is the check implementation of definitions.Coverage.
type Coverage definitions.Coverage

func (c *Coverage) GetDescription() string {
	return "enforces minimum test coverage on all packages"
}

func (c *Coverage) GetName() string {
	return "coverage"
}

func (c *Coverage) GetPrerequisites() []definitions.CheckPrerequisite {
	if c.UseCoveralls && IsContinuousIntegration() {
		return []definitions.CheckPrerequisite{{[]string{"goveralls", "-h"}, 2, "github.com/mattn/goveralls"}}
	}
	return nil
}

func (c *Coverage) Run(change scm.Change) error {
	profile, err := c.RunProfile(change)
	if err != nil {
		return err
	}
	log.Printf("%d functions profiled in %s", len(profile), change.Package())

	// TODO(maruel): Calculate the per package coverage and make it fail if a
	// package specific coverage level is specified and it's not high enough.
	// TODO(maruel): Calculate the sorted list only when -v is specified.
	maxLoc := 0
	maxName := 0
	for _, item := range profile {
		if item.Percent < 100. {
			if l := len(item.SourceRef()); l > maxLoc {
				maxLoc = l
			}
			if l := len(item.Name); l > maxName {
				maxName = l
			}
		}
	}
	for _, item := range profile {
		if item.Percent < 100. {
			log.Printf("%-*s %-*s %1.1f%%", maxLoc, item.SourceRef(), maxName, item.Name, item.Percent)
		}
	}
	total := profile.Coverage()
	partial := profile.PartiallyCoveredFuncs()
	if total < c.Global.MinCoverage {
		err = fmt.Errorf("coverage: %3.1f%% < %.1f%%; %d untested functions", total, c.Global.MinCoverage, partial)
	} else if c.Global.MaxCoverage > 0 && total > c.Global.MaxCoverage {
		err = fmt.Errorf("coverage: %3.1f%% > %.1f%%; %d untested functions; please update \"max_coverage\"", total, c.Global.MaxCoverage, partial)
	} else {
		log.Printf("coverage: %3.1f%% >= %.1f%%; %d untested functions", total, c.Global.MinCoverage, partial)
	}
	return err
}

func (c *Coverage) RunProfile(change scm.Change) (profile CoverageProfile, err error) {
	// go test accepts packages, not files.
	coverPkg := ""
	for i, p := range change.All().Packages() {
		if s := c.SettingsForPkg(p); s != nil && s.MinCoverage != 0 {
			if i != 0 {
				coverPkg += ","
			}
			coverPkg += p
		}
	}

	testPkgs := change.All().TestPackages()
	if len(testPkgs) == 0 {
		return nil, nil
	}

	tmpDir, err2 := ioutil.TempDir("", "pre-commit-go")
	if err2 != nil {
		return nil, err2
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
			// Maybe fallback to 'pkg + "/..."' and post process to remove
			// uninteresting directories. The rationale is that it will eventually
			// blow up the OS specific command argument length.
			args := []string{
				"go", "test", "-v", "-covermode=count", "-coverpkg", coverPkg,
				"-coverprofile", filepath.Join(tmpDir, fmt.Sprintf("test%d.cov", index)),
				testPkg,
			}
			out, exitCode, _ := capture(change.Repo(), args...)
			if exitCode != 0 {
				errs <- fmt.Errorf("%s %s failed:\n%s", strings.Join(args, " "), testPkg, out)
			}
		}(i, tp)
	}
	wg.Wait()

	select {
	case err = <-errs:
		return
	default:
	}

	// Merge the profiles. Sums all the counts.
	files, err2 := filepath.Glob(filepath.Join(tmpDir, "test*.cov"))
	if err2 != nil {
		return nil, err2
	}
	if len(files) == 0 {
		return nil, errors.New("no coverage found")
	}
	profilePath := filepath.Join(tmpDir, "profile.cov")
	f, err2 := os.Create(profilePath)
	if err2 != nil {
		f.Close()
		return nil, err2
	}
	err2 = mergeCoverage(files, f)
	if err2 != nil {
		f.Close()
		return nil, err2
	}
	if _, err = f.Seek(0, 0); err != nil {
		f.Close()
		return nil, err
	}
	profile, err = loadProfile(change, f)
	f.Close()
	if err != nil {
		return nil, err
	}
	// Sends to coveralls.io if applicable.
	if c.UseCoveralls && IsContinuousIntegration() {
		// Please send a pull request if the following doesn't work for you on your
		// favorite CI system.
		out, _, err2 := capture(change.Repo(), "goveralls", "-coverprofile", profilePath)
		// Don't fail the build.
		if err2 != nil {
			fmt.Printf("%s", out)
		}
	}
	return profile, err
}

func (c *Coverage) SettingsForPkg(testPkg string) *definitions.CoverageSettings {
	testDir := pkgToDir(testPkg)
	if settings, ok := c.PerDir[testDir]; ok {
		if settings == nil {
			settings = &definitions.CoverageSettings{}
		}
		return settings
	}
	return nil
}

// mergeCoverage merges multiple coverage profiles into out.
//
// It sums all the counts of each profile. It doesn't actually process it.
//
// Format is "file.go:XX.YY,ZZ.II J K"
// - file.go is path against GOPATH
// - XX.YY is the line/column start of the statement.
// - ZZ.II is the line/column end of the statement.
// - J is number of statements,
// - K is count.
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

// loadProfile loads the raw results of a coverage profile.
func loadProfile(change scm.Change, r io.Reader) (CoverageProfile, error) {
	rawProfile, err := cover.ParseProfiles(change, r)
	if err != nil {
		return nil, err
	}

	// Take the raw profile into a real one. This permits us to not have to
	// depend on "go tool cover" to save one process per package and reduce I/O
	// by reusing the in-memory file cache.
	pkg := change.Package()
	pkgOffset := len(pkg)
	if pkgOffset > 0 {
		pkgOffset++
	}
	out := CoverageProfile{}
	for _, profile := range rawProfile {
		// fn is in absolute package format based on $GOPATH. Transform to path.
		source := profile.FileName[pkgOffset:]
		content := change.Content(source)
		if content == nil {
			log.Printf("unknown file %s", source)
			continue
		}
		funcs, err := cover.FindFuncs(source, bytes.NewReader(content))
		if err != nil {
			log.Printf("broken file %s; %s", source, err)
			continue
		}
		// Now match up functions and profile blocks.
		for _, f := range funcs {
			// Convert a FuncExtent to a funcCovered.
			c, t := f.Coverage(profile)
			out = append(out, &FuncCovered{
				Source:  source,
				Line:    f.StartLine,
				Name:    f.FuncName,
				Count:   c,
				Total:   t,
				Percent: 100.0 * float64(c) / float64(t),
			})
		}
	}
	sort.Sort(out)
	return out, nil
}

type CoverageProfile []*FuncCovered

func (c CoverageProfile) Len() int      { return len(c) }
func (c CoverageProfile) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c CoverageProfile) Less(i, j int) bool {
	if c[i].Percent > c[j].Percent {
		return true
	}
	if c[i].Percent < c[j].Percent {
		return false
	}

	if c[i].Source < c[j].Source {
		return true
	}
	if c[i].Source > c[j].Source {
		return false
	}

	if c[i].Name < c[j].Name {
		return true
	}
	if c[i].Name > c[j].Name {
		return false
	}

	if c[i].Line < c[j].Line {
		return true
	}
	return false
}

// Coverage returns the coverage in % for this profile.
func (c CoverageProfile) Coverage() float64 {
	if total := c.TotalLines(); total != 0 {
		return 100. * float64(c.TotalCoveredLines()) / float64(total)
	}
	return 0
}

// TotalCoveredLines returns the number of lines that were covered.
func (c CoverageProfile) TotalCoveredLines() int64 {
	total := int64(0)
	for _, f := range c {
		total += f.Count
	}
	return total
}

// TotalLines returns the total number of source lines found.
func (c CoverageProfile) TotalLines() int64 {
	total := int64(0)
	for _, f := range c {
		total += f.Total
	}
	return total
}

// PartiallyCoveredFuncs returns the number of functions not completely covered.
func (c CoverageProfile) PartiallyCoveredFuncs() int {
	total := 0
	for _, f := range c {
		if f.Total != f.Count {
			total++
		}
	}
	return total
}

type FuncCovered struct {
	Source  string
	Line    int
	Name    string
	Count   int64
	Total   int64
	Percent float64
}

func (f *FuncCovered) SourceRef() string {
	return fmt.Sprintf("%s:%d", f.Source, f.Line)
}

func pkgToDir(p string) string {
	if p == "." {
		return p
	}
	return p[2:]
}
