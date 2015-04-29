// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
)

// getGitDir returns the .git directory path.
func getGitDir() (string, error) {
	gitDir, err := captureAbs("git", "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("failed to find .git dir: %s", err)
	}
	return gitDir, err
}

// chdirToGitRoot changes the current working directory to the git checkout
// root directory.
func chdirToGitRoot() error {
	gitRoot, err := captureAbs("git", "rev-parse", "--show-cdup")
	if err != nil {
		return fmt.Errorf("failed to find git checkout root")
	}
	if err := os.Chdir(gitRoot); err != nil {
		return fmt.Errorf("failed to chdir to git checkout root: %s", err)
	}
	return nil
}
