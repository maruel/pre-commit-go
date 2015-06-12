// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/maruel/ut"
)

func TestInternalCheck(t *testing.T) {
	t.Parallel()
	d, err := os.Getwd()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "scm", filepath.Base(d))
}

func TestIsMainPackage(t *testing.T) {
	t.Parallel()
	data := []struct {
		expected string
		in       string
	}{
		{"foo", "// Hi\npackage foo\n"},
		{"main", "package main\n"},
		{"", ""},
	}
	for i, line := range data {
		ut.AssertEqualIndex(t, i, line.expected, getPackageName([]byte(line.in)))
	}
}

func TestChangeEmpty(t *testing.T) {
	t.Parallel()
	r := &dummyRepo{t, "<root>"}
	files := []string{}
	allFiles := []string{}
	c := newChange(r, files, allFiles)
	ut.AssertEqual(t, r, c.Repo())
	ut.AssertEqual(t, "", c.Package())
	changed := c.Changed()
	ut.AssertEqual(t, []string(nil), changed.GoFiles())
	ut.AssertEqual(t, []string(nil), changed.Packages())
	ut.AssertEqual(t, []string(nil), changed.TestPackages())
	indirect := c.Indirect()
	ut.AssertEqual(t, []string(nil), indirect.GoFiles())
	ut.AssertEqual(t, []string(nil), indirect.Packages())
	ut.AssertEqual(t, []string(nil), indirect.TestPackages())
	all := c.All()
	ut.AssertEqual(t, []string(nil), all.GoFiles())
	ut.AssertEqual(t, []string(nil), all.Packages())
	ut.AssertEqual(t, []string(nil), all.TestPackages())
}

var commonTree = map[string]string{
	"bar/bar.go":      "package bar\nfunc Bar() int { return 1}",
	"bar/bar_test.go": "package bar",
	"foo/foo.go":      "package foo\nfunc Foo() int { return 42}",
	"main.go":         "package main\nimport \"foo\"\nfunc main() { foo.Foo() }",
	"main_test.go":    "package main",
}

func TestChangeIndirect(t *testing.T) {
	// main_test.go is indirectly affect by a/A.go.
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	root, allFiles, cleanup := makeTree(t,
		map[string]string{
			// Is changed.
			"a/a.go": "package a\nfunc Bar() int { return 1}",
			// Directly affected.
			"a/a_test.go": "package a",
			// Indirectly affected.
			"b/b.go": "package b\nimport \"a\"\nfunc Bar() int { return a.Bar() }",
			// Not affected.
			"c/c.go": "package c\nfunc Foo() { return 42 }",
			// Indirectly affected by "b".
			"c/c_test.go": "package c\nimport (\n\"b\"\n\"testing\"\n)\nfunc TestFoo(t *testing.T) int { if b.Bar() != 1 { t.FailNow() } }",
			// Not affected even if it imports "c".
			"d/d.go": "package d\nimport \"c\"\nfunc Foo() { return c.Foo() }",
			// Indirectly affected by "b".
			"main_test.go": "package main\nimport \"b\"\nfunc main() { println(b.Bar()) }",
		})
	defer cleanup()
	r := &dummyRepo{t, root}
	c := newChange(r, []string{"a/a.go"}, allFiles)
	ut.AssertEqual(t, r, c.Repo())
	ut.AssertEqual(t, "", c.Package())
	changed := c.Changed()
	ut.AssertEqual(t, []string{"a/a.go"}, changed.GoFiles())
	ut.AssertEqual(t, []string{"./a"}, changed.Packages())
	ut.AssertEqual(t, []string{"./a"}, changed.TestPackages())
	indirect := c.Indirect()
	ut.AssertEqual(t, []string{"a/a.go"}, indirect.GoFiles())
	ut.AssertEqual(t, []string{"./a", "./b"}, indirect.Packages())
	ut.AssertEqual(t, []string{".", "./a", "./c"}, indirect.TestPackages())
	all := c.All()
	ut.AssertEqual(t, allFiles, all.GoFiles())
	ut.AssertEqual(t, []string{".", "./a", "./b", "./c", "./d"}, all.Packages())
	ut.AssertEqual(t, []string{".", "./a", "./c"}, all.TestPackages())
}

func TestChangeIndirectReverse(t *testing.T) {
	// a_test.go is indirectly affect by z/z.go. Make sure the order files are
	// processed in does not affect the result.
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	root, allFiles, cleanup := makeTree(t,
		map[string]string{
			// Is changed.
			"z/z.go": "package z\nfunc Bar() int { return 1}",
			// Directly affected.
			"z/z_test.go": "package z",
			// Indirectly affected.
			"y/y.go": "package y\nimport \"z\"\nfunc Bar() int { return z.Bar() }",
			// Not affected.
			"x/x.go": "package x\nfunc Foo() { return 42 }",
			// Indirectly affected by "y".
			"x/x_test.go": "package x\nimport (\n\"y\"\n\"testing\"\n)\nfunc TestFoo(t *testing.T) int { if y.Bar() != 1 { t.FailNow() } }",
			// Not affected even if it imports "x".
			"w/w.go": "package w\nimport \"x\"\nfunc Foo() { return x.Foo() }",
			// Indirectly affected by "b".
			"a_test.go": "package main\nimport \"y\"\nfunc main() { println(y.Bar()) }",
		})
	defer cleanup()
	r := &dummyRepo{t, root}
	c := newChange(r, []string{"z/z.go"}, allFiles)
	ut.AssertEqual(t, r, c.Repo())
	ut.AssertEqual(t, "", c.Package())
	changed := c.Changed()
	ut.AssertEqual(t, []string{"z/z.go"}, changed.GoFiles())
	ut.AssertEqual(t, []string{"./z"}, changed.Packages())
	ut.AssertEqual(t, []string{"./z"}, changed.TestPackages())
	indirect := c.Indirect()
	ut.AssertEqual(t, []string{"z/z.go"}, indirect.GoFiles())
	ut.AssertEqual(t, []string{"./y", "./z"}, indirect.Packages())
	ut.AssertEqual(t, []string{".", "./x", "./z"}, indirect.TestPackages())
	all := c.All()
	ut.AssertEqual(t, allFiles, all.GoFiles())
	ut.AssertEqual(t, []string{".", "./w", "./x", "./y", "./z"}, all.Packages())
	ut.AssertEqual(t, []string{".", "./x", "./z"}, all.TestPackages())
}

func TestChangeAll(t *testing.T) {
	// All packages were affected, uses a slightly different (faster) code path.
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	root, allFiles, cleanup := makeTree(t,
		map[string]string{
			"bar/bar.go":      "package bar\nfunc Bar() int { return 1}",
			"bar/bar_test.go": "package bar",
			"foo/foo.go":      "package foo\nfunc Foo() int { return 42}",
			"main.go":         "package main\nimport \"foo\"\nfunc main() { foo.Foo() }",
			"main_test.go":    "package main",
		})
	defer cleanup()
	r := &dummyRepo{t, root}
	c := newChange(r, []string{"bar/bar.go", "foo/foo.go", "main.go"}, allFiles)
	ut.AssertEqual(t, r, c.Repo())
	ut.AssertEqual(t, "", c.Package())
	changed := c.Changed()
	ut.AssertEqual(t, []string{"bar/bar.go", "foo/foo.go", "main.go"}, changed.GoFiles())
	ut.AssertEqual(t, []string{".", "./bar", "./foo"}, changed.Packages())
	ut.AssertEqual(t, []string{".", "./bar"}, changed.TestPackages())
	indirect := c.Indirect()
	ut.AssertEqual(t, []string{"bar/bar.go", "foo/foo.go", "main.go"}, indirect.GoFiles())
	ut.AssertEqual(t, []string{".", "./bar", "./foo"}, indirect.Packages())
	ut.AssertEqual(t, []string{".", "./bar"}, indirect.TestPackages())
	all := c.All()
	ut.AssertEqual(t, []string{"bar/bar.go", "bar/bar_test.go", "foo/foo.go", "main.go", "main_test.go"}, all.GoFiles())
	ut.AssertEqual(t, []string{".", "./bar", "./foo"}, all.Packages())
	ut.AssertEqual(t, []string{".", "./bar"}, all.TestPackages())
}

func TestGetImports(t *testing.T) {
	t.Parallel()
	data := []struct {
		in      string
		pkg     string
		imports []string
	}{
		{
			"package foo\nimport \"bar\"",
			"foo",
			[]string{"bar"},
		},
		{
			"package foo\nimport \"host/user/repo\"",
			"foo",
			[]string{"host/user/repo"},
		},
		{
			"//\npackage foo\n//\n\nimport \"bar\"",
			"foo",
			[]string{"bar"},
		},
		{
			"package foo\nimport \"bar\"\nconst i = 0",
			"foo",
			[]string{"bar"},
		},
		{
			"package foo\nimport (\n\"bar\"\n)",
			"foo",
			[]string{"bar"},
		},
		{
			"package foo\nimport (\n\"bér\"\n)",
			"foo",
			[]string{"bér"},
		},
		{
			"package foo\nimport (\n\"b\u00e8r\"\n)",
			"foo",
			[]string{"bèr"},
		},
		{
			// This is not legal Go.
			"package foo\nimport (\n\"bér\" \"ber\"\n)",
			"foo",
			[]string{"bér", "ber"},
		},
		{
			"package foo\nimport (\n\"bar\"\n)\nconst i = 0",
			"foo",
			[]string{"bar"},
		},
		{
			"package foo\nimport (\n\t\"bar\"\n\n\t// Yo\n\"baz\"\n  )",
			"foo",
			[]string{"bar", "baz"},
		},
	}

	for i, line := range data {
		pkg, imports := getImports([]byte(line.in))
		ut.AssertEqualIndex(t, i, line.pkg, pkg)
		ut.AssertEqualIndex(t, i, line.imports, imports)
	}
}

// Private stuff.

type dummyRepo struct {
	t    *testing.T
	root string
}

func (d *dummyRepo) Root() string              { return d.root }
func (d *dummyRepo) ScmDir() (string, error)   { d.t.FailNow(); return "", nil }
func (d *dummyRepo) HookPath() (string, error) { d.t.FailNow(); return "", nil }
func (d *dummyRepo) HEAD() Commit              { d.t.FailNow(); return "" }
func (d *dummyRepo) Ref() string               { d.t.FailNow(); return "" }
func (d *dummyRepo) Upstream() (Commit, error) { d.t.FailNow(); return "", nil }
func (d *dummyRepo) Between(recent, old Commit, ignoredPaths []string) (Change, error) {
	d.t.FailNow()
	return nil, nil
}

// makeTree creates a temporary directory and creates the files in it.
//
// Returns the root directly, all files created and the cleanup function.
func makeTree(t *testing.T, files map[string]string) (string, []string, func()) {
	td, err := ioutil.TempDir("", "pre-commit-go")
	ut.AssertEqual(t, nil, err)
	cleanup := func() {
		if err := os.RemoveAll(td); err != nil {
			t.Fail()
		}
	}
	allFiles := make([]string, 0, len(files))
	for f, c := range files {
		allFiles = append(allFiles, f)
		p := filepath.Join(td, f)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			cleanup()
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(p, []byte(c), 0600); err != nil {
			cleanup()
			t.Fatal(err)
		}
	}
	sort.Strings(allFiles)
	return td, allFiles, cleanup
}
