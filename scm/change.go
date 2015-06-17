// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"go/scanner"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Change represents a change to test against.
//
// This interface is specialized for Go projects.
type Change interface {
	// Repo references back to the repository.
	Repo() ReadOnlyRepo
	// Package returns the package name to reference Repo().Root(). Returns an
	// empty string if the repository is located outside of $GOPATH.
	Package() string
	// Changed is the directly affected files and packages.
	Changed() Set
	// Indirect returns the Set of everything affected indirectly, e.g. all
	// modified files plus all packages importing a package that was modified by
	// this Change. It is useful for example to run all tests that could be
	// indirectly impacted by a change. Indirect().GoFiles() ==
	// Changed().GoFiles(). Only Packages() and TestPackages() can be longer, up
	// to values returned by All()'s.
	Indirect() Set
	// All returns all the files in the repository.
	All() Set
	// Content returns the content of a file.
	Content(name string) []byte
	// IsIgnored returns true if this path is ignored. This is mostly relevant
	// when using tools that work at the package level instead of at the file
	// level and generated files (like proto-gen-go generated files) should be
	// ignored.
	IsIgnored(p string) bool
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
	repo           ReadOnlyRepo
	packageName    string
	ignorePatterns IgnorePatterns
	direct         set
	indirect       set
	all            set

	lock    sync.Mutex
	content map[string][]byte
}

func newChange(r ReadOnlyRepo, files, allFiles, ignorePatterns IgnorePatterns) *change {
	//log.Printf("Change{%s, %s}", files, allFiles)
	root := r.Root()
	// An error occurs when the repository is not inside GOPATH. Ignore this
	// error here.
	pkgName, _ := relToGOPATH(root, r.GOPATH())
	c := &change{
		repo:           r,
		packageName:    pkgName,
		ignorePatterns: ignorePatterns,
		content:        map[string][]byte{},
	}

	// Map of <relative directory> : <relative package>
	testDirs := map[string]string{}
	sourceDirs := map[string]string{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		c.direct.files = append(c.direct.files, f)
		dir := dirName(f)
		if _, ok := sourceDirs[dir]; !ok {
			relPkgName := dirToPkg(dir)
			sourceDirs[dir] = relPkgName
			c.direct.packages = append(c.direct.packages, relPkgName)
		}
		if strings.HasSuffix(f, "_test.go") {
			if _, ok := testDirs[dir]; !ok {
				relPkgName := dirToPkg(dir)
				testDirs[dir] = relPkgName
				c.direct.testPackages = append(c.direct.testPackages, relPkgName)
			}
		}
	}

	// Map of <relative directory> : <all files in this directory>
	allDirs := map[string][]string{}
	// Set of <relative directory>
	allTestDirs := map[string]bool{}
	allSourceDirs := map[string]bool{}
	// Map of <absolute package name> : <relative directory>
	allPkgs := map[string]string{}
	for _, f := range allFiles {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		c.all.files = append(c.all.files, f)
		dir := dirName(f)
		allDirs[dir] = append(allDirs[dir], filepath.Base(f))
		if _, ok := allSourceDirs[dir]; !ok {
			relPkgName := dirToPkg(dir)
			allSourceDirs[dir] = true
			c.all.packages = append(c.all.packages, relPkgName)
			allPkgs[path.Join(pkgName, strings.Replace(dir, pathSeparator, "/", -1))] = dir
		}
		if strings.HasSuffix(f, "_test.go") {
			if _, ok := allTestDirs[dir]; !ok {
				relPkgName := dirToPkg(dir)
				allTestDirs[dir] = true
				c.all.testPackages = append(c.all.testPackages, relPkgName)
				if _, ok = testDirs[dir]; !ok {
					if _, ok = sourceDirs[dir]; ok {
						// Add test packages where only non-test source files were modified.
						testDirs[dir] = relPkgName
						c.direct.testPackages = append(c.direct.testPackages, relPkgName)
					}
				}
			}
		}
	}

	// Still need to sort these since "." will not be at the right place.
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		sort.Strings(c.direct.packages)
	}()
	go func() {
		defer wg.Done()
		sort.Strings(c.direct.testPackages)
	}()
	go func() {
		defer wg.Done()
		sort.Strings(c.all.packages)
	}()
	go func() {
		defer wg.Done()
		sort.Strings(c.all.testPackages)
	}()
	wg.Wait()

	c.indirect.files = c.direct.files
	if len(c.direct.packages) == len(c.all.packages) && len(c.direct.testPackages) == len(c.all.testPackages) {
		// Everything is affected. Skip processing files.
		c.indirect.packages = c.direct.packages
		c.indirect.testPackages = c.direct.testPackages
	} else {
		c.indirect.packages = make([]string, len(c.direct.packages))
		copy(c.indirect.packages, c.direct.packages)
		c.indirect.testPackages = make([]string, len(c.direct.testPackages))
		copy(c.indirect.testPackages, c.direct.testPackages)
		// First create a maps of what imports what. Then resolve the DAG. Go spec
		// guarantees it's a DAG. Treat tests independently since they are not
		// subject to the DAG restriction; tests can't be imported so they do not
		// affect other packages indirectly.

		// Map <imported relative dir> : <set of relative dirs importing this package>
		reverseImports := map[string]map[string]bool{}
		reverseTestImports := map[string]map[string]bool{}
		// Parallelize but rate limited. The goal is to work around the os.Open()
		// file latency, especially on Windows.
		parallel := make(chan bool, 16)
		for i := 0; i < cap(parallel); i++ {
			parallel <- true
		}
		var wg sync.WaitGroup
		for baseDir, files := range allDirs {
			if _, ok := sourceDirs[baseDir]; ok {
				// Already in indirect.
				continue
			}
			for _, f := range files {
				wg.Add(1)
				go func(baseDir, f string) {
					<-parallel
					defer func() {
						wg.Done()
						parallel <- true
					}()
					content := c.Content(filepath.Join(baseDir, f))
					if content == nil {
						return
					}
					_, localImports := getImports(content)
					for _, imp := range localImports {
						if importedDir, ok := allPkgs[imp]; ok {
							isTest := strings.HasSuffix(f, "_test.go")
							c.lock.Lock()
							if !isTest {
								if reverseImports[importedDir] == nil {
									reverseImports[importedDir] = map[string]bool{}
								}
								reverseImports[importedDir][baseDir] = true
							} else {
								if reverseTestImports[importedDir] == nil {
									reverseTestImports[importedDir] = map[string]bool{}
								}
								reverseTestImports[importedDir][baseDir] = true
							}
							c.lock.Unlock()
						}
					}
				}(baseDir, f)
			}
		}
		wg.Wait()

		// First resolve imports. Do it iteratively, so it's exponential runtime.
		// Reimplement with better algo once the runtime is >5ms.
		found := true
		for found {
			found = false
			for dir := range sourceDirs {
				for importerDir := range reverseImports[dir] {
					if _, ok := sourceDirs[importerDir]; !ok {
						relPkgName := dirToPkg(importerDir)
						sourceDirs[importerDir] = relPkgName
						c.indirect.packages = append(c.indirect.packages, relPkgName)
						found = true

						// Does it contain tests too?
						if _, ok := allTestDirs[importerDir]; ok {
							if _, ok = testDirs[importerDir]; !ok {
								testDirs[importerDir] = relPkgName
								c.indirect.testPackages = append(c.indirect.testPackages, relPkgName)
							}
						}
					}
				}
			}
		}

		// Tests export nothing, so no need to do a multi-pass.
		for dir := range sourceDirs {
			for importerDir := range reverseTestImports[dir] {
				if _, ok := testDirs[importerDir]; !ok {
					relPkgName := dirToPkg(importerDir)
					testDirs[importerDir] = relPkgName
					c.indirect.testPackages = append(c.indirect.testPackages, relPkgName)
				}
			}
		}
		wg.Add(2)
		go func() {
			defer wg.Done()
			sort.Strings(c.indirect.packages)
		}()
		go func() {
			defer wg.Done()
			sort.Strings(c.indirect.testPackages)
		}()
		wg.Wait()
	}
	return c
}

func (c *change) Repo() ReadOnlyRepo {
	return c.repo
}

func (c *change) Package() string {
	return c.packageName
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

func (c *change) Content(p string) []byte {
	c.lock.Lock()
	content, ok := c.content[p]
	c.lock.Unlock()
	if !ok {
		var err error
		content, err = ioutil.ReadFile(filepath.Join(c.repo.Root(), p))
		if err != nil {
			log.Printf("failed to read %s: %s", p, err)
		}
		c.lock.Lock()
		c.content[p] = content
		c.lock.Unlock()
	}
	return content
}

func (c *change) IsIgnored(p string) bool {
	return c.ignorePatterns.Match(p)
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

func dirToPkg(d string) string {
	if d == "." {
		return d
	}
	return "./" + strings.Replace(d, pathSeparator, "/", -1)
}

func dirName(p string) string {
	if d := filepath.Dir(p); d != "" {
		return d
	}
	return "."
}

// getPackageName returns the name of the package as defined as a
// "package foo" statement.
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

// getImports returns the package name and all imports of a file.
//
// It is similar to go/parser.ParseFile() with mode ImportsOnly. This function
// is an extremely simplified version which doesn't construct an ast.File tree,
// mainly for performance.
//
// Switch back to ParseFile() if bugs are found.
func getImports(content []byte) (string, []string) {
	// As per https://golang.org/ref/spec, ignoring comments, package must happen
	// first, then import statements (potentially multiple) and only then the
	// actual code exists. So processing cuts short after the first non-import
	// statement.
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(content))
	s.Init(file, content, nil, 0)
	pkgName := ""
	var imports []string
outer:
	for {
		_, tok, _ := s.Scan()
		switch tok {
		case token.ILLEGAL, token.EOF:
			// Likely a truncated file.
			break outer

		case token.PACKAGE:
			_, tok, lit := s.Scan()
			if tok == token.IDENT {
				pkgName = lit
			} else {
				panic("Temporary")
			}

		case token.IMPORT:
			// Scan all the following lines.
			for {
				_, tok, lit := s.Scan()
				switch tok {
				case token.STRING:
					imports = append(imports, lit[1:len(lit)-1])

				case token.LPAREN, token.IMPORT, token.RPAREN, token.SEMICOLON:
					// There can be multiple imports statement, they can have
					// parenthesis, semicolon is implicitly added after each line

				case token.PERIOD:
					// '.' can be used to import everything inline.

				case token.IDENT:
					// A named package. It can be "_" or anything else.

				case token.EOF:
					// A file with only import statement but no code.
					break outer

				default:
					// Any other statement breaks the loop.
					break outer
				}
			}

		case token.COMMENT:
		case token.SEMICOLON:

		default:
			// This happens if a source file does not import anything. For example
			// the file only defines constants or pure algorithm.
			break
		}
	}
	return pkgName, imports
}
