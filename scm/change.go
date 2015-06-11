// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Change represents a change to test against.
//
// This interface is specialized for Go projects.
type Change interface {
	// Repo references back to the repository.
	Repo() ReadOnlyRepo
	// Changed is the directly affected files and packages.
	Changed() Set
	// Indirect returns the Set of everything affected indirectly, e.g. all
	// modified files plus all packages importing a package that was modified by
	// this Change. It is useful for example to run all tests that could be
	// indirectly impacted by a change.
	Indirect() Set
	// All returns all the files in the repository.
	All() Set
}

// Set is a subset of files/directories/packages relative to the change and the
// overall repository.
type Set interface {
	// GoFiles returns all the source files, including tests.
	GoFiles() []string
	// Packages returns all the packages included in this set, using the relative
	// notation, e.g. with prefix "./" relative to the checkout root. So this
	// package "scm" would be represented as "./scm".
	Packages() []string
	// TestPackages returns all the packages included in this set that contain
	// tests, using the relative notation, e.g. with prefix "./".
	//
	// In summary, it is the same result as Packages() but without the ones with
	// no test.
	TestPackages() []string
}

// Private details.

const pathSeparator = string(os.PathSeparator)

type change struct {
	repo        ReadOnlyRepo
	packageName string
	direct      set
	indirect    set
	all         set
}

func newChange(repo ReadOnlyRepo, files, allFiles []string) *change {
	// An error occurs when the repository is not inside GOPATH. Ignore this
	// error here.
	p, _ := relToGOPATH(repo.Root())
	c := &change{repo: repo, packageName: p}
	makeSet(&c.direct, files)
	// TODO(maruel): Actually indirect.
	makeSet(&c.indirect, allFiles)
	makeSet(&c.all, allFiles)
	return c
}

func (c *change) Repo() ReadOnlyRepo {
	return c.repo
}

func (c *change) Changed() Set {
	return &c.direct
}

func (c *change) Indirect() Set {
	return &c.indirect
}

func (c *change) All() Set {
	return &c.all
}

type set struct {
	files        []string
	packages     []string
	testPackages []string
}

func (s *set) GoFiles() []string {
	return s.files
}

func (s *set) Packages() []string {
	return s.packages
}

func (s *set) TestPackages() []string {
	return s.testPackages
}

func makeSet(s *set, files []string) {
	testDirs := map[string]bool{}
	sourceDirs := map[string]bool{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		s.files = append(s.files, f)
		dir := filepath.Dir(f)
		if _, ok := sourceDirs[dir]; !ok {
			sourceDirs[dir] = true
			s.packages = append(s.packages, dirToPkg(dir))
		}
		if strings.HasSuffix(f, "_test.go") {
			if _, ok := testDirs[dir]; !ok {
				testDirs[dir] = true
				s.testPackages = append(s.testPackages, dirToPkg(dir))
			}
		}
	}
	sort.Strings(s.files)
	sort.Strings(s.packages)
	sort.Strings(s.testPackages)
}

func dirToPkg(d string) string {
	if d == "." {
		return d
	}
	return "./" + strings.Replace(d, pathSeparator, "/", -1)
}

/*
	// Only scan the first file per directory.
	if _, ok := dirsPackageFound[dir]; !ok {
		if content, err := ioutil.ReadFile(p); err == nil {
			name := getPackageName(content)
			dirsPackageFound[dir] = name != "main" && name != ""
		}
	}
*/

func getPackageName(content []byte) string {
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(content))
	s.Init(file, content, nil, 0)
	for {
		_, tok, _ := s.Scan()
		if tok == token.EOF {
			return ""
		}
		if tok == token.PACKAGE {
			_, tok, lit := s.Scan()
			if tok == token.IDENT {
				return lit
			}
		}
	}
}
