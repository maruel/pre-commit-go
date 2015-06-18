// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/maruel/pre-commit-go/checks/definitions"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/ut"
)

func TestCoverageGlobal(t *testing.T) {
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
		UseGlobalInference: true,
		UseCoveralls:       false,
		Global: definitions.CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDirDefault: definitions.CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDir: map[string]*definitions.CoverageSettings{},
	}
	profile, err := c.RunProfile(change)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, 55.555555555555555, profile.CoveragePercent())
	ut.AssertEqual(t, 2, profile.PartiallyCoveredFuncs())
	expected := CoverageProfile{
		{
			Source:    "foo.go",
			Line:      2,
			SourceRef: "foo.go:2",
			Name:      "Foo",
			Count:     1,
			Total:     1,
			Percent:   100,
		},
		{
			Source:    "bar/bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar/bar.go",
			Line:      10,
			SourceRef: "bar/bar.go:10",
			Name:      "Baz",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile)

	expected = CoverageProfile{
		{
			Source:    "bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar.go",
			Line:      10,
			SourceRef: "bar/bar.go:10",
			Name:      "Baz",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile.Subset("bar"))

	expected = CoverageProfile{
		{
			Source:    "foo.go",
			Line:      2,
			SourceRef: "foo.go:2",
			Name:      "Foo",
			Count:     1,
			Total:     1,
			Percent:   100,
		},
	}
	ut.AssertEqual(t, expected, profile.Subset("."))
}

func TestCoverageLocal(t *testing.T) {
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
		UseGlobalInference: false,
		UseCoveralls:       false,
		Global: definitions.CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDirDefault: definitions.CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDir: map[string]*definitions.CoverageSettings{},
	}
	profile, err := c.RunProfile(change)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, 55.555555555555555, profile.CoveragePercent())
	ut.AssertEqual(t, 2, profile.PartiallyCoveredFuncs())
	expected := CoverageProfile{
		{
			Source:    "foo.go",
			Line:      3,
			SourceRef: "foo.go:3",
			Name:      "Type.Foo",
			Count:     1,
			Total:     1,
			Percent:   100,
		},
		{
			Source:    "bar/bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar/bar.go",
			Line:      10,
			SourceRef: "bar/bar.go:10",
			Name:      "Baz",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile)

	expected = CoverageProfile{
		{
			Source:    "bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar.go",
			Line:      10,
			SourceRef: "bar/bar.go:10",
			Name:      "Baz",
			Count:     2,
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile.Subset("bar"))

	ut.AssertEqual(t, nil, c.Run(change))
}

func TestCoveragePrerequisites(t *testing.T) {
	// This test can't be parallel.
	if !IsContinuousIntegration() {
		old := os.Getenv("CI")
		defer func() {
			ut.ExpectEqual(t, nil, os.Setenv("CI", old))
		}()
		ut.AssertEqual(t, nil, os.Setenv("CI", "true"))
		ut.AssertEqual(t, true, IsContinuousIntegration())
	}
	c := Coverage{UseCoveralls: true}
	ut.AssertEqual(t, 1, len(c.GetPrerequisites()))
}

func TestCoverageEmpty(t *testing.T) {
	t.Parallel()
	ut.AssertEqual(t, 0., CoverageProfile{}.CoveragePercent())
	c := Coverage{PerDir: map[string]*definitions.CoverageSettings{"foo": nil}}
	ut.AssertEqual(t, &definitions.CoverageSettings{}, c.SettingsForPkg("foo"))
}

var coverageFiles = map[string]string{
	"foo.go": `package foo
type Type int
func (i Type) Foo() int {
  return 1
}
`,
	"foo_test.go": `package foo
import "testing"
func TestSuccess(t *testing.T) {
	f := Type(0)
  if f.Foo() != 1 {
    t.Fail()
  }
}
`,
	"bar/bar.go": `package bar
func Bar(i int) int {
	if i == 2 {
		return 2
	}
	i++
	return i
}

func Baz(i int) int {
	if i == 2 {
		return 2
	}
	i++
	return i
}
`,
	"bar/bar_test.go": `package bar
import "testing"
func TestSuccess(t *testing.T) {
  if Bar(2) != 2 {
    t.Fail()
  }
  if Baz(2) != 2 {
    t.Fail()
  }
}
`,
}
