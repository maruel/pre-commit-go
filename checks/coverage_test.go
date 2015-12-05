// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/maruel/pre-commit-go/Godeps/_workspace/src/github.com/maruel/ut"
	"github.com/maruel/pre-commit-go/internal"
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
		Global: CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDirDefault: CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDir: map[string]*CoverageSettings{},
	}
	profile, err := c.RunProfile(change, &Options{MaxDuration: 1})
	ut.AssertEqual(t, nil, err)
	expected := CoverageProfile{
		{
			Source:    "foo.go",
			Line:      3,
			SourceRef: "foo.go:3",
			Name:      "Type.Foo",
			Covered:   2,
			Missing:   []int{},
			Total:     2,
			Percent:   100,
		},
		{
			Source:    "bar/bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Covered:   2,
			Missing:   []int{7, 8},
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar/bar.go",
			Line:      11,
			SourceRef: "bar/bar.go:11",
			Name:      "Baz",
			Covered:   2,
			Missing:   []int{16, 17},
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile)
	ut.AssertEqual(t, 60., profile.CoveragePercent())
	ut.AssertEqual(t, 2, profile.PartiallyCoveredFuncs())

	expected = CoverageProfile{
		{
			Source:    "bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Covered:   2,
			Missing:   []int{7, 8},
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar.go",
			Line:      11,
			SourceRef: "bar/bar.go:11",
			Name:      "Baz",
			Covered:   2,
			Missing:   []int{16, 17},
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile.Subset("bar"))

	expected = CoverageProfile{
		{
			Source:    "foo.go",
			Line:      3,
			SourceRef: "foo.go:3",
			Name:      "Type.Foo",
			Covered:   2,
			Missing:   []int{},
			Total:     2,
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
		Global: CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDirDefault: CoverageSettings{
			MinCoverage: 50,
			MaxCoverage: 100,
		},
		PerDir: map[string]*CoverageSettings{},
	}
	profile, err := c.RunProfile(change, &Options{MaxDuration: 1})
	ut.AssertEqual(t, nil, err)
	expected := CoverageProfile{
		{
			Source:    "foo.go",
			Line:      3,
			SourceRef: "foo.go:3",
			Name:      "Type.Foo",
			Covered:   2,
			Missing:   []int{},
			Total:     2,
			Percent:   100,
		},
		{
			Source:    "bar/bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Covered:   2,
			Missing:   []int{7, 8},
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar/bar.go",
			Line:      11,
			SourceRef: "bar/bar.go:11",
			Name:      "Baz",
			Covered:   2,
			Missing:   []int{16, 17},
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile)
	ut.AssertEqual(t, 60., profile.CoveragePercent())
	ut.AssertEqual(t, 2, profile.PartiallyCoveredFuncs())

	expected = CoverageProfile{
		{
			Source:    "bar.go",
			Line:      2,
			SourceRef: "bar/bar.go:2",
			Name:      "Bar",
			Covered:   2,
			Missing:   []int{7, 8},
			Total:     4,
			Percent:   50,
		},
		{
			Source:    "bar.go",
			Line:      11,
			SourceRef: "bar/bar.go:11",
			Name:      "Baz",
			Covered:   2,
			Missing:   []int{16, 17},
			Total:     4,
			Percent:   50,
		},
	}
	ut.AssertEqual(t, expected, profile.Subset("bar"))

	ut.AssertEqual(t, nil, c.Run(change, &Options{MaxDuration: 1}))
}

var coverageFiles = map[string]string{
	"foo.go": `package foo
type Type int
func (i *Type) Foo() int {

	j := int(*i) // 5
	return j+1
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

	if j := i; j == 2 {  // 3 is reported instead of 4 because the coverage is 2~4.
		return 2  // 5
	}
	i++
	return i
}
// 10
func Baz(i int) int {
	if i == 2 {
		return 2
	}
	// 15 Random comment.
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
	c := Coverage{PerDir: map[string]*CoverageSettings{"foo": nil}}
	ut.AssertEqual(t, &CoverageSettings{}, c.SettingsForPkg("foo"))
}

func TestRangeToString(t *testing.T) {
	t.Parallel()
	ut.AssertEqual(t, "", rangeToString(nil))
	ut.AssertEqual(t, "1", rangeToString([]int{1}))
	ut.AssertEqual(t, "1-2", rangeToString([]int{1, 2}))
	ut.AssertEqual(t, "1-3", rangeToString([]int{1, 2, 3}))
	ut.AssertEqual(t, "1,3-4,6-8", rangeToString([]int{1, 3, 4, 6, 7, 8}))
	ut.AssertEqual(t, "1,3-4,6", rangeToString([]int{1, 3, 4, 6}))
}
