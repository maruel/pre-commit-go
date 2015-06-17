// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/maruel/pre-commit-go/checks/definitions"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
	"github.com/maruel/ut"
)

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
		if name == "custom" {
			c = &custom{
				Description: "foo",
				Command:     []string{"go", "version"},
				Prerequisites: []definitions.CheckPrerequisite{
					{
						HelpCommand:      []string{"go", "version"},
						ExpectedExitCode: 0,
						URL:              "example.com.local",
					},
				},
			}
		}
		if l, ok := c.(sync.Locker); ok {
			l.Lock()
			l.Unlock()
		}
		if err := c.Run(change); err != nil {
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
		// TODO(maruel): Make golint and govet fail. They are not currently working
		// at all.
		if name == "custom" || name == "golint" || name == "govet" {
			continue
		}
		if err := c.Run(change); err == nil {
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
	p := []definitions.CheckPrerequisite{
		{
			HelpCommand:      []string{"go", "version"},
			ExpectedExitCode: 0,
			URL:              "example.com.local",
		},
	}
	c := &custom{
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
	"foo.go": `// Foo

// +build: incorrect

package foo

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
		for _, name := range getKnownChecks() {
			for _, p := range KnownChecks[name]().GetPrerequisites() {
				out, _, _ := internal.Capture("", nil, "go", "get", p.URL)
				if len(out) != 0 {
					// This is essentially a race condition, ignore failure but log it.
					fmt.Printf("prerequisite %s installation failed: %s", p.URL, out)
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
	_, code, err := internal.Capture(fooDir, nil, "git", "init")
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)
	// It's important to add the files to the index, otherwise they will be
	// ignored.
	_, code, err = internal.Capture(fooDir, nil, "git", "add", ".")
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)

	repo, err := scm.GetRepo(fooDir, td)
	ut.AssertEqual(t, nil, err)
	change, err := repo.Between(scm.Current, scm.GitInitialCommit, nil)
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
