// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/ut"
)

func TestCoverage(t *testing.T) {
	// Can't run in parallel due to os.Chdir() and os.Setenv().
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
	oldGOPATH := os.Getenv("GOPATH")
	defer func() {
		ut.ExpectEqual(t, nil, os.Setenv("GOPATH", oldGOPATH))
	}()
	ut.AssertEqual(t, nil, os.Setenv("GOPATH", td))

	oldWd, change := setup(t, td, coverageFiles)
	defer func() {
		ut.ExpectEqual(t, nil, os.Chdir(oldWd))
	}()

	c := &Coverage{
		MinCoverage:  50,
		MaxCoverage:  100,
		UseCoveralls: false,
	}
	profile, err := c.RunProfile(change)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, 100., profile.Coverage())
	ut.AssertEqual(t, 0, profile.PartiallyCoveredFuncs())
	expected := CoverageProfile{
		{
			Source:  "bar/bar.go",
			Line:    2,
			Name:    "Bar",
			Count:   1,
			Total:   1,
			Percent: 100,
		},
		{
			Source:  "foo.go",
			Line:    2,
			Name:    "Foo",
			Count:   1,
			Total:   1,
			Percent: 100,
		},
	}
	ut.AssertEqual(t, expected, profile)
	ut.AssertEqual(t, "bar/bar.go:2", profile[0].SourceRef())
	ut.AssertEqual(t, "foo.go:2", profile[1].SourceRef())
	ut.AssertEqual(t, nil, c.Run(change))
}

var coverageFiles = map[string]string{
	"foo.go": `package foo
func Foo() int {
  return 1
}
`,
	"foo_test.go": `package foo
import "testing"
func TestSuccess(t *testing.T) {
  if Foo() != 1 {
    t.Fail()
  }
}
`,
	"bar/bar.go": `package bar
func Bar() int {
	return 2
}
`,
	"bar/bar_test.go": `package bar
import "testing"
func TestSuccess(t *testing.T) {
  if Bar() != 2 {
    t.Fail()
  }
}
`,
}
