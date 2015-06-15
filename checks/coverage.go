// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// coverage is a large check so it is in its own file.

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
	if c.UseCoveralls && IsContinuousIntegration() {
		toInstall = append(toInstall, definitions.CheckPrerequisite{[]string{"goveralls", "-h"}, 2, "github.com/mattn/goveralls"})
	}
	return toInstall
}

func (c *coverage) Run(change scm.Change) (err error) {
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

	// Merge the profiles. Sums all the counts.
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

	profile, total, partial, err := loadProfile(change, profilePath)
	if err != nil {
		return err
	}
	log.Printf("%d functions profiled in %s", len(profile), coverPkg)

	// TODO(maruel): Calculate the sorted list only when -v is specified.
	maxLoc := 0
	maxName := 0
	sort.Sort(profile)
	for _, item := range profile {
		if item.percent < 100. {
			if l := len(item.sourceRef()); l > maxLoc {
				maxLoc = l
			}
			if l := len(item.name); l > maxName {
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
		if item.percent < 100. {
			log.Printf("%-*s %-*s %1.1f%%", maxLoc, item.sourceRef()[offset:], maxName, item.name, item.percent)
		}
	}
	if total < c.MinCoverage {
		err = fmt.Errorf("code coverage: %3.1f%% < %.1f%%; %d untested functions", total, c.MinCoverage, partial)
	} else if c.MaxCoverage > 0 && total > c.MaxCoverage {
		err = fmt.Errorf("code coverage: %3.1f%% > %.1f%%; %d untested functions; please update \"max_coverage\"", total, c.MaxCoverage, partial)
	} else {
		log.Printf("code coverage: %3.1f%% >= %.1f%%; %d untested functions", total, c.MinCoverage, partial)
	}
	if err == nil {
		select {
		case err = <-errs:
		default:
		}
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
			return nil, 0, 0, fmt.Errorf("malformed coverage file")
		}
		if loc == "total" {
			// TODO(maruel): This value must be ignored and recalculated, since some
			// files may be ignored.
			total = percent
		} else {
			items := rsplitn(loc, ":", 2)
			if len(items) != 2 {
				return nil, 0, 0, fmt.Errorf("malformed coverage file")
			}
			if change.IsIgnored(items[0]) {
				// This file is ignored.
				continue
			}
			lineNum, err := strconv.ParseInt(items[1], 10, 32)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("malformed coverage file")
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
	if c[i].percent > c[j].percent {
		return true
	}
	if c[i].percent < c[j].percent {
		return false
	}

	if c[i].source < c[j].source {
		return true
	}
	if c[i].source > c[j].source {
		return false
	}

	if c[i].name < c[j].name {
		return true
	}
	if c[i].name > c[j].name {
		return false
	}

	if c[i].line < c[j].line {
		return true
	}
	return false
}

type funcCovered struct {
	percent float64
	source  string
	line    int
	name    string
}

func (f *funcCovered) sourceRef() string {
	return fmt.Sprintf("%s:%d", f.source, f.line)
}
