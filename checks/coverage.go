// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// coverage is a large check so it is in its own file.
//
// It is designed to be usable standalone.

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

// Coverage is the check implementation of definitions.Coverage.
type Coverage definitions.Coverage

func (c *Coverage) GetDescription() string {
	return "enforces minimum test coverage on all packages"
}

func (c *Coverage) GetName() string {
	return "coverage"
}

func (c *Coverage) GetPrerequisites() []definitions.CheckPrerequisite {
	toInstall := []definitions.CheckPrerequisite{
		{[]string{"go", "tool", "cover", "-h"}, 1, "golang.org/x/tools/cmd/cover"},
	}
	if c.UseCoveralls && IsContinuousIntegration() {
		toInstall = append(toInstall, definitions.CheckPrerequisite{[]string{"goveralls", "-h"}, 2, "github.com/mattn/goveralls"})
	}
	return toInstall
}

func (c *Coverage) Run(change scm.Change) error {
	profile, total, partial, err := c.RunProfile(change)
	if err != nil {
		return err
	}
	log.Printf("%d functions profiled in %s", len(profile), change.Package())

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
	offset := len(change.Package())
	if offset > 0 {
		offset++
		maxLoc -= offset
	}
	for _, item := range profile {
		if item.Percent < 100. {
			log.Printf("%-*s %-*s %1.1f%%", maxLoc, item.SourceRef()[offset:], maxName, item.Name, item.Percent)
		}
	}
	if total < c.MinCoverage {
		err = fmt.Errorf("coverage: %3.1f%% < %.1f%%; %d untested functions", total, c.MinCoverage, partial)
	} else if c.MaxCoverage > 0 && total > c.MaxCoverage {
		err = fmt.Errorf("coverage: %3.1f%% > %.1f%%; %d untested functions; please update \"max_coverage\"", total, c.MaxCoverage, partial)
	} else {
		log.Printf("coverage: %3.1f%% >= %.1f%%; %d untested functions", total, c.MinCoverage, partial)
	}
	return err
}

func (c *Coverage) RunProfile(change scm.Change) (profile coverageProfile, total float64, partial int, err error) {
	// go test accepts packages, not files.
	coverPkg := ""
	for i, p := range change.All().Packages() {
		tmp := p
		if tmp != "." {
			tmp = tmp[2:]
		}
		for _, ignore := range c.SkipDirs {
			if tmp == ignore {
				goto skip
			}
		}
		if i != 0 {
			coverPkg += ","
		}
		coverPkg += p
	skip:
	}

	testPkgs := change.All().TestPackages()
	if len(testPkgs) == 0 {
		return nil, 0, 0, nil
	}

	tmpDir, err2 := ioutil.TempDir("", "pre-commit-go")
	if err2 != nil {
		return nil, 0, 0, err2
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
			out, exitCode, _ := internal.Capture("", nil, args...)
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
		return nil, 0, 0, err2
	}
	if len(files) == 0 {
		return nil, 0, 0, errors.New("no coverage found")
	}
	profilePath := filepath.Join(tmpDir, "profile.cov")
	f, err2 := os.Create(profilePath)
	if err2 != nil {
		return nil, 0, 0, err2
	}
	err2 = mergeCoverage(files, f)
	f.Close()
	if err2 != nil {
		return nil, 0, 0, err2
	}

	profile, total, partial, err = loadProfile(change, profilePath)
	if err != nil {
		return nil, 0, 0, err
	}
	// Sends to coveralls.io if applicable.
	if c.UseCoveralls && IsContinuousIntegration() {
		// Please send a pull request if the following doesn't work for you on your
		// favorite CI system.
		out, _, err2 := internal.Capture("", nil, "goveralls", "-coverprofile", profilePath)
		// Don't fail the build.
		if err2 != nil {
			fmt.Printf("%s", out)
		}
	}
	sort.Sort(profile)
	return profile, total, partial, err
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
func loadProfile(change scm.Change, p string) (coverageProfile, float64, int, error) {
	out, code, err := internal.Capture("", nil, "go", "tool", "cover", "-func", p)
	if code != 0 || err != nil {
		return nil, 0, 0, fmt.Errorf("go tool cover failed with code %d; %s", code, err)
	}
	profile := coverageProfile{}
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
			return nil, 0, 0, fmt.Errorf("malformed coverage file: %s", line)
		}
		if loc == "total" {
			// TODO(maruel): This value must be ignored and recalculated, since some
			// files may be ignored.
			total = percent
		} else {
			items := rsplitn(loc, ":", 2)
			if len(items) != 2 {
				return nil, 0, 0, fmt.Errorf("malformed coverage file: %s", line)
			}
			if change.IsIgnored(items[0]) {
				// This file is ignored.
				continue
			}
			lineNum, err := strconv.ParseInt(items[1], 10, 32)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("malformed coverage file: %s", line)
			}
			profile = append(profile, funcCovered{percent, items[0], int(lineNum), name})
			if percent < 100. {
				partial++
			}
		}
	}
	return profile, total, partial, nil
}

type coverageProfile []funcCovered

func (c coverageProfile) Len() int      { return len(c) }
func (c coverageProfile) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c coverageProfile) Less(i, j int) bool {
	if c[i].Percent > c[j].Percent {
		return true
	}
	if c[i].Percent < c[j].Percent {
		return false
	}

	if c[i].source < c[j].source {
		return true
	}
	if c[i].source > c[j].source {
		return false
	}

	if c[i].Name < c[j].Name {
		return true
	}
	if c[i].Name > c[j].Name {
		return false
	}

	if c[i].line < c[j].line {
		return true
	}
	return false
}

type funcCovered struct {
	Percent float64
	source  string
	line    int
	Name    string
}

func (f *funcCovered) SourceRef() string {
	return fmt.Sprintf("%s:%d", f.source, f.line)
}
