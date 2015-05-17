// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"fmt"
	"path/filepath"

	"github.com/maruel/pre-commit-go/internal"
)

type Repo interface {
	Root() string
	PreCommitHookPath() (string, error)
}

// GetRepo returns a valid Repo if one is found.
func GetRepo() (Repo, error) {
	// TODO(maruel): Accept cwd.
	root, err := internal.CaptureAbs("git", "rev-parse", "--show-cdup")
	if err == nil {
		return &git{root: root}, nil
	}
	// TODO: Add your favorite SCM.
	return nil, fmt.Errorf("failed to find git checkout root")
}

type git struct {
	root   string
	gitDir string
}

func (g *git) Root() string {
	return g.root
}

func (g *git) PreCommitHookPath() (string, error) {
	if g.gitDir == "" {
		var err error
		g.gitDir, err = getGitDir()
		if err != nil {
			return "", fmt.Errorf("failed to find .git dir: %s", err)
		}
	}
	return filepath.Join(g.gitDir, "hooks", "pre-commit"), nil
}

// getGitDir returns the .git directory path.
func getGitDir() (string, error) {
	gitDir, err := internal.CaptureAbs("git", "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("failed to find .git dir: %s", err)
	}
	return gitDir, err
}
