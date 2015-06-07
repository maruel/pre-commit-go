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

	"github.com/maruel/ut"
)

func TestSuccess(t *testing.T) {
	// Runs all checks, they should all pass.
	if testing.Short() {
		t.SkipNow()
	}
	defer func() {
		goDirsCache = nil
	}()
	td, err := ioutil.TempDir("", "pre-commit-go")
	ut.AssertEqual(t, nil, err)
	defer func() {
		if err := os.RemoveAll(td); err != nil {
			t.Fail()
		}
	}()
	oldWd, _ := setup(t, td, goodFiles)
	defer func() {
		ut.ExpectEqual(t, nil, os.Chdir(oldWd))
	}()
	oldGOPATH := os.Getenv("GOPATH")
	defer func() {
		ut.ExpectEqual(t, nil, os.Setenv("GOPATH", oldGOPATH))
	}()
	ut.AssertEqual(t, nil, os.Setenv("GOPATH", td))
	for _, name := range getKnownChecks() {
		c := KnownChecks[name]
		// TODO(maruel): Fix errcheck locally.
		if name == "custom" || name == "errcheck" {
			continue
		}
		if err := c.Run(); err != nil {
			t.Errorf("%s failed: %s", c.GetName(), err)
		}
	}
}

func TestChecksFailure(t *testing.T) {
	// Runs all checks, they should all fail.
	if testing.Short() {
		t.SkipNow()
	}
	defer func() {
		goDirsCache = nil
	}()
	td, err := ioutil.TempDir("", "pre-commit-go")
	ut.AssertEqual(t, nil, err)
	defer func() {
		if err := os.RemoveAll(td); err != nil {
			t.Fail()
		}
	}()
	oldWd, _ := setup(t, td, badFiles)
	defer func() {
		ut.ExpectEqual(t, nil, os.Chdir(oldWd))
	}()
	oldGOPATH := os.Getenv("GOPATH")
	defer func() {
		ut.ExpectEqual(t, nil, os.Setenv("GOPATH", oldGOPATH))
	}()
	ut.AssertEqual(t, nil, os.Setenv("GOPATH", td))
	for _, name := range getKnownChecks() {
		c := KnownChecks[name]
		// TODO(maruel): Make golint and govet fail.
		if name == "custom" || name == "golint" || name == "govet" {
			continue
		}
		if err := c.Run(); err == nil {
			t.Errorf("%s didn't fail but was expected to", c.GetName())
		}
	}
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

package foo

// Syntax error:
func main() {
`,
	"foo_test.go": `// Foo

package foo

import "testing"

func MissingDesc() {
}

func TestFail(t *testing.T) {
t.Fail()
}
`,
}

func setup(t *testing.T, td string, files map[string]string) (string, string) {
	goDirsCache = nil
	fooDir := filepath.Join(td, "src", "foo")
	ut.AssertEqual(t, nil, os.MkdirAll(fooDir, 0700))
	for f, c := range files {
		ioutil.WriteFile(filepath.Join(fooDir, f), []byte(c), 0600)
	}
	oldWd, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, os.Chdir(fooDir))
	return oldWd, fooDir
}

func getKnownChecks() []string {
	names := make([]string, 0, len(KnownChecks))
	for name := range KnownChecks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}