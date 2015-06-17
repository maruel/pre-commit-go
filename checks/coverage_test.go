// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"io/ioutil"
	"testing"

	"github.com/maruel/pre-commit-go/checks/definitions"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/ut"
)

func TestCoverage(t *testing.T) {
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
	change := setup(t, td, coverageFiles)

	c := &Coverage{
		Global: definitions.CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDirDefault: definitions.CoverageSettings{
			MinCoverage: 0,
			MaxCoverage: 0,
		},
		UseCoveralls: false,
		PerDir:       map[string]*definitions.CoverageSettings{},
	}
	profile, err := c.RunProfile(change)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, 75., profile.Coverage())
	ut.AssertEqual(t, 1, profile.PartiallyCoveredFuncs())
	expected := CoverageProfile{
		{
			Source:  "foo.go",
			Line:    2,
			Name:    "Foo",
			Count:   1,
			Total:   1,
			Percent: 100,
		},
		{
			Source:  "bar/bar.go",
			Line:    2,
			Name:    "Bar",
			Count:   2,
			Total:   3,
			Percent: 66.666666666666666,
		},
	}
	ut.AssertEqual(t, expected, profile)
	ut.AssertEqual(t, "foo.go:2", profile[0].SourceRef())
	ut.AssertEqual(t, "bar/bar.go:2", profile[1].SourceRef())
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
func Bar(i int) int {
	if i == 2 {
		return 2
	}
	return 3
}
`,
	"bar/bar_test.go": `package bar
import "testing"
func TestSuccess(t *testing.T) {
  if Bar(2) != 2 {
    t.Fail()
  }
}
`,
}
