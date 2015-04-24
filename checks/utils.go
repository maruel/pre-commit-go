// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"fmt"
	"go/scanner"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
)

// Globals

var goDirsCacheLock sync.Mutex
var goDirsCache map[dirsType][]string

var relToGOPATHLock sync.Mutex
var relToGOPATHCache = map[string]string{}

type dirsType int

const (
	sourceDirs  dirsType = 0 // Directories containing go source files.
	testDirs    dirsType = 1 // Directories containing tests are returned.
	packageDirs dirsType = 2 // Directories containing non "main" packages.
)

func readDirNames(dirname string) []string {
	f, err := os.Open(dirname)
	if err != nil {
		return nil
	}
	names, err := f.Readdirnames(-1)
	_ = f.Close()
	return names
}

// captureWd runs an executable from a directory returns the output, exit code
// and error if appropriate.
func captureWd(wd string, args ...string) (string, int, error) {
	exitCode := -1
	log.Printf("capture(%s)", args)
	c := exec.Command(args[0], args[1:]...)
	if wd != "" {
		c.Dir = wd
	}
	out, err := c.CombinedOutput()
	if c.ProcessState != nil {
		if waitStatus, ok := c.ProcessState.Sys().(syscall.WaitStatus); ok {
			exitCode = waitStatus.ExitStatus()
			if exitCode != 0 {
				err = nil
			}
		}
	}
	// TODO(maruel): Handle code page on Windows.
	return string(out), exitCode, err
}

// capture runs an executable and returns the output, exit code and error if
// appropriate.
func capture(args ...string) (string, int, error) {
	return captureWd("", args...)
}

// reverse reverses a string.
func reverse(s string) string {
	n := len(s)
	runes := make([]rune, n)
	for _, rune := range s {
		n--
		runes[n] = rune
	}
	return string(runes[n:])
}

func rsplitn(s, sep string, n int) []string {
	items := strings.SplitN(reverse(s), sep, n)
	l := len(items)
	for i := 0; i < l/2; i++ {
		j := l - i - 1
		items[i], items[j] = reverse(items[j]), reverse(items[i])
	}
	if l&1 != 0 {
		i := l / 2
		items[i] = reverse(items[i])
	}
	return items
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

// goDirs returns the list of directories with '*.go' files or '*_test.go'
// files.
//
// If 'tests' is true, all directories containing tests are returned.
// If 'tests' is false, only directories containing go source files but not
// tests are returned. This is usually 'main' packages.
func goDirs(d dirsType) []string {
	goDirsCacheLock.Lock()
	defer goDirsCacheLock.Unlock()
	if goDirsCache != nil {
		return goDirsCache[d]
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
	goDirsCache = map[dirsType][]string{
		sourceDirs:  make([]string, 0, len(dirsSourceFound)),
		testDirs:    make([]string, 0, len(dirsTestsFound)),
		packageDirs: {},
	}
	for d := range dirsSourceFound {
		goDirsCache[sourceDirs] = append(goDirsCache[sourceDirs], d)
	}
	for d := range dirsTestsFound {
		goDirsCache[testDirs] = append(goDirsCache[testDirs], d)
	}
	for d, v := range dirsPackageFound {
		if v {
			goDirsCache[packageDirs] = append(goDirsCache[packageDirs], d)
		}
	}
	sort.Strings(goDirsCache[sourceDirs])
	sort.Strings(goDirsCache[testDirs])
	sort.Strings(goDirsCache[packageDirs])
	return goDirsCache[d]
}

// relToGOPATH returns the path relative to $GOPATH/src.
func relToGOPATH(p string) (string, error) {
	relToGOPATHLock.Lock()
	defer relToGOPATHLock.Unlock()
	if rel, ok := relToGOPATHCache[p]; ok {
		return rel, nil
	}
	for _, gopath := range filepath.SplitList(os.Getenv("GOPATH")) {
		if len(gopath) == 0 {
			continue
		}
		srcRoot := filepath.Join(gopath, "src")
		// TODO(maruel): Accept case-insensitivity on Windows/OSX, maybe call
		// filepath.EvalSymlinks().
		if !strings.HasPrefix(p, srcRoot) {
			continue
		}
		rel, err := filepath.Rel(srcRoot, p)
		if err != nil {
			return "", fmt.Errorf("failed to find relative path from %s to %s", srcRoot, p)
		}
		relToGOPATHCache[p] = rel
		//log.Printf("relToGOPATH(%s) = %s", p, rel)
		return rel, err
	}
	return "", fmt.Errorf("failed to find GOPATH relative directory for %s", p)
}
