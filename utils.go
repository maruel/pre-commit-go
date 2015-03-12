// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
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
var goDirsCache map[bool][]string

var relToGOPATHLock sync.Mutex
var relToGOPATHCache = map[string]string{}

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

// captureAbs returns an absolute path of whatever a git command returned.
func captureAbs(args ...string) (string, error) {
	out, code, _ := capture(args...)
	if code != 0 {
		return "", fmt.Errorf("failed to run \"%s\"", strings.Join(args, " "))
	}
	path, err := filepath.Abs(strings.TrimSpace(out))
	log.Printf("captureAbs(%s) = %s", args, path)
	return path, err
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

// goDirs returns the list of directories with '*.go' files or '*_test.go'
// files, depending on value of 'tests'.
func goDirs(tests bool) []string {
	goDirsCacheLock.Lock()
	defer goDirsCacheLock.Unlock()
	if goDirsCache != nil {
		return goDirsCache[tests]
	}
	root, _ := os.Getwd()
	if stat, err := os.Stat(root); err != nil || !stat.IsDir() {
		panic("internal failure")
	}

	dirsSourceFound := map[string]bool{}
	dirsTestsFound := map[string]bool{}
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
				}
			}
		}
	}
	recurse(root)
	goDirsCache = map[bool][]string{
		false: make([]string, 0, len(dirsSourceFound)),
		true:  make([]string, 0, len(dirsTestsFound)),
	}
	for d := range dirsSourceFound {
		goDirsCache[false] = append(goDirsCache[false], d)
	}
	for d := range dirsTestsFound {
		goDirsCache[true] = append(goDirsCache[true], d)
	}
	sort.Strings(goDirsCache[false])
	sort.Strings(goDirsCache[true])
	//log.Printf("goDirs() = %v", goDirsCache)
	return goDirsCache[tests]
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
