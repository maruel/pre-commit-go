// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/maruel/pre-commit-go/Godeps/_workspace/src/github.com/maruel/ut"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
)

func init() {
	for _, i := range []string{"GIT_WORK_TREE", "GIT_DIR", "GIT_PREFIX"} {
		_ = os.Unsetenv(i)
	}
}

func TestCheckPrerequisite(t *testing.T) {
	// Runs all checks, they should all pass.
	t.Parallel()
	ut.AssertEqual(t, true, (&CheckPrerequisite{HelpCommand: []string{"go", "version"}, ExpectedExitCode: 0}).IsPresent())
	ut.AssertEqual(t, false, (&CheckPrerequisite{HelpCommand: []string{"go", "version"}, ExpectedExitCode: 1}).IsPresent())
}

func TestChecksSuccess(t *testing.T) {
	// Runs all checks, they should all pass.
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	td, err := ioutil.TempDir("", "pre-commit-go")
	ut.AssertEqual(t, nil, err)
	defer func() {
		if err := internal.RemoveAll(td); err != nil {
			t.Fail()
		}
	}()
	change := setup(t, td, goodFiles)
	for _, name := range getKnownChecks() {
		c := KnownChecks[name]()
		switch name {
		case "custom":
			c = &Custom{
				Description:   "foo",
				Command:       []string{"go", "version"},
				CheckExitCode: true,
				Prerequisites: []CheckPrerequisite{
					{
						HelpCommand:      []string{"go", "version"},
						ExpectedExitCode: 0,
						URL:              "example.com.local",
					},
				},
			}
		case "copyright":
			cop := c.(*Copyright)
			cop.Header = "// Foo"
		case "coverage":
			cov := c.(*Coverage)
			cov.Global.MinCoverage = 100
			cov.Global.MaxCoverage = 100
			cov.PerDirDefault.MinCoverage = 100
			cov.PerDirDefault.MaxCoverage = 100
		}
		if l, ok := c.(sync.Locker); ok {
			l.Lock()
			l.Unlock()
		}
		if err := c.Run(change, &Options{MaxDuration: 1}); err != nil {
			t.Errorf("%s failed: %s", c.GetName(), err)
		}
	}
}

func TestChecksFailure(t *testing.T) {
	// Runs all checks, they should all fail.
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	td, err := ioutil.TempDir("", "pre-commit-go")
	ut.AssertEqual(t, nil, err)
	defer func() {
		if err := internal.RemoveAll(td); err != nil {
			t.Fail()
		}
	}()
	change := setup(t, td, badFiles)
	for _, name := range getKnownChecks() {
		c := KnownChecks[name]()
		switch name {
		case "custom":
			c = &Custom{
				Description:   "foo",
				Command:       []string{"go", "invalid"},
				CheckExitCode: true,
				Prerequisites: []CheckPrerequisite{
					{
						HelpCommand:      []string{"go", "version"},
						ExpectedExitCode: 0,
						URL:              "example.com.local",
					},
				},
			}
		case "copyright":
			cop := c.(*Copyright)
			cop.Header = "// Expected header"
		case "coverage":
			cov := c.(*Coverage)
			cov.Global.MinCoverage = 100
			cov.Global.MaxCoverage = 100
			cov.PerDirDefault.MinCoverage = 100
			cov.PerDirDefault.MaxCoverage = 100
		}
		if err := c.Run(change, &Options{MaxDuration: 1}); err == nil {
			t.Errorf("%s didn't fail but was expected to", c.GetName())
		}
	}
}

func TestChecksDescriptions(t *testing.T) {
	t.Parallel()
	for _, name := range getKnownChecks() {
		c := KnownChecks[name]()
		ut.AssertEqual(t, true, c.GetDescription() != "")
		c.GetPrerequisites()
	}
}

func TestCustom(t *testing.T) {
	t.Parallel()
	p := []CheckPrerequisite{
		{
			HelpCommand:      []string{"go", "version"},
			ExpectedExitCode: 0,
			URL:              "example.com.local",
		},
	}
	c := &Custom{
		Description:   "foo",
		Command:       []string{"go", "version"},
		Prerequisites: p,
	}
	ut.AssertEqual(t, "foo", c.GetDescription())
	ut.AssertEqual(t, p, c.GetPrerequisites())
}

// Private stuff.

// This set of files passes all the tests.
var goodFiles = map[string]string{
	"foo.go": `// Foo

package foo

// Foo returns 1.
func Foo() int {
	return 1
}
`,
	"foo_test.go": `// Foo

package foo

import "testing"

func TestSuccess(t *testing.T) {
	if Foo() != 1 {
		t.Fail()
	}
}
`,
}

// This set of files fails all the tests.
var badFiles = map[string]string{
	"foo.go": "// Foo\n\n// +build: incorrect\n\npackage foo\n" + `
import "errors"

// bad description.
func MissingDesc() {
	// Error starts with upper case and ends with a dot.
	return errors.New("Bad error.")
}

func main() {
}
`,
	"foo_test.go": `// Foo

package foo

import "testing"

func TestFail(t *testing.T) {
t.Fail()
}
`,
}

func init() {
	if IsContinuousIntegration() {
		// The reason it's being done is that "go test" starts before prerequisites
		// are installed. But since we are testing prerequisites, this runs in
		// conflict. Wait for prerequisites to be installed.
		loop := true
		for loop {
			loop = false
			for _, name := range getKnownChecks() {
				for _, p := range KnownChecks[name]().GetPrerequisites() {
					if !p.IsPresent() {
						time.Sleep(10 * time.Millisecond)
						loop = true
						break
					}
				}
			}
		}
	}
}

func setup(t *testing.T, td string, files map[string]string) scm.Change {
	fooDir := filepath.Join(td, "src", "foo")
	ut.AssertEqual(t, nil, os.MkdirAll(fooDir, 0700))
	for f, c := range files {
		p := filepath.Join(fooDir, f)
		ut.AssertEqual(t, nil, os.MkdirAll(filepath.Dir(p), 0700))
		ut.AssertEqual(t, nil, ioutil.WriteFile(p, []byte(c), 0600))
	}
	out, code, err := internal.Capture(fooDir, nil, "git", "init")
	ut.AssertEqualf(t, 0, code, out)
	ut.AssertEqual(t, nil, err)
	// It's important to add the files to the index, otherwise they will be
	// ignored.
	out, code, err = internal.Capture(fooDir, nil, "git", "add", ".")
	ut.AssertEqualf(t, 0, code, out)
	ut.AssertEqual(t, nil, err)

	repo, err := scm.GetRepo(fooDir, td)
	ut.AssertEqual(t, nil, err)
	change, err := repo.Between(scm.Current, scm.Initial, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, true, change != nil)
	return change
}

func getKnownChecks() []string {
	names := make([]string, 0, len(KnownChecks))
	for name := range KnownChecks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
