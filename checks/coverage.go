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
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
	if c.isGoverallsEnabled() {
		return []definitions.CheckPrerequisite{{[]string{"goveralls", "-h"}, 2, "github.com/mattn/goveralls"}}
	}
	return nil
}

func (c *Coverage) Run(change scm.Change) error {
	profile, err := c.RunProfile(change)
	if err != nil {
		return err
	}
	out, err := ProcessProfile(profile, &c.Global)
	log.Printf("Results:\n%s", out)
	if err != nil {
		return fmt.Errorf("coverage: %s", err)
	}
	return nil
}

// RunProfile runs a coverage run according to the settings and return results.
func (c *Coverage) RunProfile(change scm.Change) (profile CoverageProfile, err error) {
	// go test accepts packages, not files.
	testPkgs := change.All().TestPackages()
	if len(testPkgs) == 0 {
		// Sir, there's no test.
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

	profile, err = c.RunGlobal(change, tmpDir)
	if err != nil {
		return nil, err
	}

	if c.isGoverallsEnabled() {
		// Please send a pull request if the following doesn't work for you on your
		// favorite CI system.
		out, _, err2 := capture(change.Repo(), "goveralls", "-coverprofile", filepath.Join(tmpDir, "profile.cov"))
		// Don't fail the build.
		if err2 != nil {
			fmt.Printf("%s", out)
		}
	}
	return profile, nil
}

// RunGlobal runs the tests under coverage with global inference.
//
// This means that test can contribute coverage in any other package, even
// outside their own package.
func (c *Coverage) RunGlobal(change scm.Change, tmpDir string) (CoverageProfile, error) {
	coverPkg := ""
	for i, p := range change.All().Packages() {
		if s := c.SettingsForPkg(p); s != nil && s.MinCoverage != 0 {
			if i != 0 {
				coverPkg += ","
			}
			coverPkg += p
		}
	}

	// This part is similar to Test.Run() except that it passes a unique
	// -coverprofile file name, so that all the files can later be merged into a
	// single file.
	testPkgs := change.All().TestPackages()
	type result struct {
		file string
		err  error
	}
	results := make(chan *result)
	for index, tp := range testPkgs {
		f := filepath.Join(tmpDir, fmt.Sprintf("test%d.cov", index))
		go func(f string, testPkg string) {
			// Maybe fallback to 'pkg + "/..."' and post process to remove
			// uninteresting directories. The rationale is that it will eventually
			// blow up the OS specific command argument length.
			args := []string{
				"go", "test", "-v", "-covermode=count", "-coverpkg", coverPkg,
				"-coverprofile", f,
				testPkg,
			}
			out, exitCode, err := capture(change.Repo(), args...)
			if exitCode != 0 {
				err = fmt.Errorf("%s %s failed:\n%s", strings.Join(args, " "), testPkg, out)
			}
			results <- &result{f, err}
		}(f, tp)
	}

	// Sends to coveralls.io if applicable. Do not write to disk unless needed.
	var f readWriteSeekCloser
	var err error
	profilePath := filepath.Join(tmpDir, "profile.cov")
	if c.isGoverallsEnabled() {
		if f, err = os.Create(profilePath); err != nil {
			return nil, err
		}
	} else {
		f = &buffer{}
	}

	// Aggregate all results.
	counts := map[string]int{}
	for i := 0; i < len(testPkgs); i++ {
		result := <-results
		if err != nil {
			continue
		}
		if result.err != nil {
			err = result.err
			continue
		}
		if err2 := loadRawCoverage(result.file, counts); err == nil {
			// Wait for all tests to complete before returning.
			err = err2
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return loadMergeAndClose(f, counts, change)
}

// SettingsForPkg returns the settings for a particular package.
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

func (c *Coverage) isGoverallsEnabled() bool {
	return c.UseCoveralls && IsContinuousIntegration()
}

// ProcessProfile generates output that can be optionally printed and an error if the check failed.
func ProcessProfile(profile CoverageProfile, settings *definitions.CoverageSettings) (string, error) {
	out := ""
	maxLoc := 0
	maxName := 0
	for _, item := range profile {
		if item.Percent < 100. {
			if l := len(item.SourceRef); l > maxLoc {
				maxLoc = l
			}
			if l := len(item.Name); l > maxName {
				maxName = l
			}
		}
	}
	for _, item := range profile {
		if item.Percent < 100. {
			out += fmt.Sprintf("%-*s %-*s %4.1f%% (%d/%d)\n", maxLoc, item.SourceRef, maxName, item.Name, item.Percent, item.Count, item.Total)
		}
	}
	if err := profile.Passes(settings); err != nil {
		return out, err
	}
	out += fmt.Sprintf(
		"coverage: %3.1f%% (%d/%d) >= %.1f%%; Functions: %d untested / %d partially / %d completely\n",
		profile.CoveragePercent(), profile.TotalCoveredLines(), profile.TotalLines(), settings.MinCoverage, profile.NonCoveredFuncs(), profile.PartiallyCoveredFuncs(), profile.CoveredFuncs())
	return out, nil
}

// CoverageProfile is the processed results of a coverage run.
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

// Subset returns a new CoverageProfile that only covers the specified
// directory.
func (c CoverageProfile) Subset(p string) CoverageProfile {
	p += "/"
	out := CoverageProfile{}
	for _, i := range c {
		if strings.HasPrefix(i.Source, p) {
			rest := i.Source[len(p):]
			if !strings.Contains(rest, "/") {
				j := *i
				j.Source = rest
				out = append(out, &j)
			}
		}
	}
	return out
}

// Passes returns nil if it passes the settings otherwise returns an error.
func (c CoverageProfile) Passes(s *definitions.CoverageSettings) error {
	total := c.CoveragePercent()
	if total < s.MinCoverage {
		return fmt.Errorf("%3.1f%% (%d/%d) < %.1f%%; Functions: %d untested / %d partially / %d completely",
			total, c.TotalCoveredLines(), c.TotalLines(), s.MinCoverage, c.NonCoveredFuncs(), c.PartiallyCoveredFuncs(), c.CoveredFuncs())
	}
	if s.MaxCoverage > 0 && total > s.MaxCoverage {
		return fmt.Errorf("%3.1f%% (%d/%d) > %.1f%%; Functions: %d untested / %d partially / %d completely",
			total, c.TotalCoveredLines(), c.TotalLines(), s.MaxCoverage, c.NonCoveredFuncs(), c.PartiallyCoveredFuncs(), c.CoveredFuncs())
	}
	return nil
}

// Coverage returns the coverage in % for this profile.
func (c CoverageProfile) CoveragePercent() float64 {
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

// NonCoveredFuncs returns the number of functions not covered.
func (c CoverageProfile) NonCoveredFuncs() int {
	total := 0
	for _, f := range c {
		if f.Count == 0 {
			total++
		}
	}
	return total
}

// PartiallyCoveredFuncs returns the number of functions partially covered.
func (c CoverageProfile) PartiallyCoveredFuncs() int {
	total := 0
	for _, f := range c {
		if f.Count != 0 && f.Total != f.Count {
			total++
		}
	}
	return total
}

// CoveredFuncs returns the number of functions completely covered.
func (c CoverageProfile) CoveredFuncs() int {
	total := 0
	for _, f := range c {
		if f.Total == f.Count {
			total++
		}
	}
	return total
}

// FuncCovered is the summary of a function covered.
type FuncCovered struct {
	Source    string
	Line      int
	SourceRef string
	Name      string
	Count     int64
	Total     int64
	Percent   float64
}

// Private stuff.

func pkgToDir(p string) string {
	if p == "." {
		return p
	}
	return p[2:]
}

type readWriteSeekCloser interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
}

// buffer implements readWriteSeekCloser.
type buffer struct {
	bytes.Buffer
}

func (b *buffer) Close() error {
	return nil
}

func (b *buffer) Seek(i int64, j int) (int64, error) {
	if i != 0 || j != 0 {
		panic("internal bug")
	}
	return 0, nil
}

// loadMergeAndClose calls mergeCoverage() then loadProfile().
func loadMergeAndClose(f readWriteSeekCloser, counts map[string]int, change scm.Change) (CoverageProfile, error) {
	defer f.Close()
	err := mergeCoverage(counts, f)
	if err != nil {
		return nil, err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	return loadProfile(change, f)
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
func mergeCoverage(counts map[string]int, out io.Writer) error {
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

// loadRawCoverage loads a coverage profile file without any interpretation.
func loadRawCoverage(file string, counts map[string]int) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	// Strip the first line.
	s.Scan()
	if line := s.Text(); line != "mode: count" {
		return fmt.Errorf("malformed %s: %s", file, line)
	}
	for s.Scan() {
		line := s.Text()
		items := rsplitn(line, " ", 2)
		if len(items) != 2 {
			return fmt.Errorf("malformed %s", file)
		}
		if items[0] == "total:" {
			// Skip last line.
			continue
		}
		count, err := strconv.Atoi(items[1])
		if err != nil {
			break
		}
		counts[items[0]] += int(count)
	}
	return err
}

// loadProfile loads the raw results of a coverage profile.
//
// It is already pre-sorted.
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
				Source:    source,
				Line:      f.StartLine,
				SourceRef: fmt.Sprintf("%s:%d", source, f.StartLine),
				Name:      f.FuncName,
				Count:     c,
				Total:     t,
				Percent:   100.0 * float64(c) / float64(t),
			})
		}
	}
	sort.Sort(out)
	return out, nil
}
