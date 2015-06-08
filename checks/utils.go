// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// IsContinuousIntegration returns true if it thinks it's running on a known CI
// service.
func IsContinuousIntegration() bool {
	// Refs:
	// - http://docs.travis-ci.com/user/environment-variables/
	// - http://docs.drone.io/env.html
	// - https://circleci.com/docs/environment-variables
	return os.Getenv("CI") == "true"
}

// Globals

var relToGOPATHLock sync.Mutex
var relToGOPATHCache = map[string]string{}

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
