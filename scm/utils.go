// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// relToGOPATH returns the path relative to $GOPATH/src.
func relToGOPATH(p string) (string, error) {
	for _, gopath := range filepath.SplitList(os.Getenv("GOPATH")) {
		if len(gopath) == 0 {
			continue
		}
		srcRoot := filepath.Join(gopath, "src")
		// TODO(maruel): Accept case-insensitivity on Windows/OSX, maybe call
		// filepath.EvalSymlinks().
		// Calling EvalSymlinks() is a bad idea, as some projects (e.g.
		// circleci.com) like to checkout outside of $GOPATH then symlink back in.
		if !strings.HasPrefix(p, srcRoot) {
			continue
		}
		rel, err := filepath.Rel(srcRoot, p)
		if err != nil {
			return "", fmt.Errorf("failed to find relative path from %s to %s", srcRoot, p)
		}
		return rel, err
	}
	return "", fmt.Errorf("failed to find GOPATH relative directory for %s", p)
}
