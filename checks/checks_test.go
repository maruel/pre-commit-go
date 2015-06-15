// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

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
		if err := os.RemoveAll(td); err != nil {
			t.Fail()
		}
	}()
	oldWd, change := setup(t, td, goodFiles)
	defer func() {
		ut.ExpectEqual(t, nil, os.Chdir(oldWd))
	}()
	oldGOPATH := os.Getenv("GOPATH")
	defer func() {
		ut.ExpectEqual(t, nil, os.Setenv("GOPATH", oldGOPATH))
	}()
	ut.AssertEqual(t, nil, os.Setenv("GOPATH", td))
	waitForDependencies(t)
	for _, name := range getKnownChecks() {
		c := KnownChecks[name]()
		if name == "custom" {
			continue
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
		if err := os.RemoveAll(td); err != nil {
			t.Fail()
		}
	}()
	oldWd, change := setup(t, td, badFiles)
	defer func() {
		ut.ExpectEqual(t, nil, os.Chdir(oldWd))
	}()
	oldGOPATH := os.Getenv("GOPATH")
	defer func() {
		ut.ExpectEqual(t, nil, os.Setenv("GOPATH", oldGOPATH))
	}()
	ut.AssertEqual(t, nil, os.Setenv("GOPATH", td))
	waitForDependencies(t)
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

func TestChecks(t *testing.T) {
	t.Parallel()
	waitForDependencies(t)
}

// Private stuff.

func waitForDependencies(t *testing.T) {
	// That is totally cheezy. It's because this test assumes that all
	// prerequisites have been installed, which is false. Probably worth getting
	// rid of this check since it's a problem on the CI.
	for {
		success := true
		for _, name := range getKnownChecks() {
			c := KnownChecks[name]()
			for _, p := range c.GetPrerequisites() {
				if !p.IsPresent() {
					success = false
				}
			}
			ut.AssertEqual(t, true, c.GetDescription() != "")
		}
		if !IsContinuousIntegration() || success {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}

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

func setup(t *testing.T, td string, files map[string]string) (string, scm.Change) {
	fooDir := filepath.Join(td, "src", "foo")
	ut.AssertEqual(t, nil, os.MkdirAll(fooDir, 0700))
	for f, c := range files {
		ut.AssertEqual(t, nil, ioutil.WriteFile(filepath.Join(fooDir, f), []byte(c), 0600))
	}
	oldWd, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, os.Chdir(fooDir))
	_, code, err := internal.Capture(fooDir, nil, "git", "init")
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)
	// It's important to add the files to the index, otherwise they will be
	// ignored.
	_, code, err = internal.Capture(fooDir, nil, "git", "add", ".")
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)

	repo, err := scm.GetRepo(fooDir)
	ut.AssertEqual(t, nil, err)
	change, err := repo.Between(scm.Current, scm.GitInitialCommit, nil)
	ut.AssertEqual(t, nil, err)
	return oldWd, change
}

func getKnownChecks() []string {
	names := make([]string, 0, len(KnownChecks))
	for name := range KnownChecks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
