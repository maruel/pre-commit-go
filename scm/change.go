// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package scm implements repository management.
package scm

import (
	"go/scanner"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Change represents a change to test against.
type Change interface {
	Repo() Repo
	SourceDirs() []string
	TestDirs() []string
	PackageDirs() []string
}

// Private details.

type dirsType int

const (
	sourceDirs  dirsType = 0 // Directories containing go source files.
	testDirs    dirsType = 1 // Directories containing tests are returned.
	packageDirs dirsType = 2 // Directories containing non "main" packages.
)

type change struct {
	repo        Repo
	lock        sync.Mutex
	files       []string
	allFiles    []string
	goDirsCache map[dirsType][]string
}

func (c *change) Repo() Repo {
	return c.repo
}

func (c *change) SourceDirs() []string {
	return c.goDirs(sourceDirs)
}

func (c *change) TestDirs() []string {
	return c.goDirs(testDirs)
}

func (c *change) PackageDirs() []string {
	return c.goDirs(packageDirs)
}

// goDirs returns the list of directories with '*.go' files or '*_test.go'
// files.
//
// If 'tests' is true, all directories containing tests are returned.
// If 'tests' is false, only directories containing go source files but not
// tests are returned. This is usually 'main' packages.
func (c *change) goDirs(d dirsType) []string {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.goDirsCache != nil {
		return c.goDirsCache[d]
	}
	root, _ := os.Getwd()
	if stat, err := os.Stat(root); err != nil || !stat.IsDir() {
		panic("internal failure")
	}

	// A directory can be in all 3.
	dirsSourceFound := map[string]bool{}
	dirsTestsFound := map[string]bool{}
	dirsPackageFound := map[string]bool{}
	var recurse func(dir string)
	recurse = func(dir string) {
		for _, f := range readDirNames(dir) {
			if f[0] == '.' || f[0] == '_' {
				continue
			}
			p := filepath.Join(dir, f)
			stat, err := os.Stat(p)
			if err != nil {
				continue
			}
			if stat.IsDir() {
				recurse(p)
			} else {
				if strings.HasSuffix(p, "_test.go") {
					dirsTestsFound[dir] = true
				} else if strings.HasSuffix(p, ".go") {
					dirsSourceFound[dir] = true
					// Only scan the first file per directory.
					if _, ok := dirsPackageFound[dir]; !ok {
						if content, err := ioutil.ReadFile(p); err == nil {
							name := getPackageName(content)
							dirsPackageFound[dir] = name != "main" && name != ""
						}
					}
				}
			}
		}
	}
	recurse(root)
	c.goDirsCache = map[dirsType][]string{
		sourceDirs:  make([]string, 0, len(dirsSourceFound)),
		testDirs:    make([]string, 0, len(dirsTestsFound)),
		packageDirs: {},
	}
	for d := range dirsSourceFound {
		c.goDirsCache[sourceDirs] = append(c.goDirsCache[sourceDirs], d)
	}
	for d := range dirsTestsFound {
		c.goDirsCache[testDirs] = append(c.goDirsCache[testDirs], d)
	}
	for d, v := range dirsPackageFound {
		if v {
			c.goDirsCache[packageDirs] = append(c.goDirsCache[packageDirs], d)
		}
	}
	sort.Strings(c.goDirsCache[sourceDirs])
	sort.Strings(c.goDirsCache[testDirs])
	sort.Strings(c.goDirsCache[packageDirs])
	return c.goDirsCache[d]
}

func readDirNames(dirname string) []string {
	f, err := os.Open(dirname)
	if err != nil {
		return nil
	}
	names, err := f.Readdirnames(-1)
	_ = f.Close()
	return names
}

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
