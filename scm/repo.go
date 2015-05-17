// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maruel/pre-commit-go/internal"
)

// Repo represents a source control management checkout.
type Repo interface {
	// Root returns the root directory of this repository.
	Root() string
	// PreCommitHookPath returns the path to the bash script called as part of a
	// commit.
	PreCommitHookPath() (string, error)
	// HEAD returns the HEAD commit hash.
	HEAD() string
	// Untracked returns the list of untracked files.
	Untracked() ([]string, error)
	// Unstaged returns the list with changes not in the staging index.
	Unstaged() ([]string, error)
	// Stash stashes the content that is not in the index.
	Stash() (bool, error)
	// Stash restores the stash generated from Stash.
	Restore() error
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

func (g *git) HEAD() string {
	if _, _, err := g.capture("rev-parse", "--verify", "HEAD"); err == nil {
		return "HEAD"
	}
	// Initial commit: diff against an empty tree object
	return "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
}

func (g *git) Untracked() ([]string, error) {
	out, _, err := g.capture("ls-files", "--others", "--exclude-standard")
	return strings.Split(out, "\n"), err
}

func (g *git) Unstaged() ([]string, error) {
	out, _, err := g.capture("diff", "--name-only", "--no-color", "--no-ext-diff")
	return strings.Split(out, "\n"), err
}

func (g *git) Stash() (bool, error) {
	if untracked, err := g.Untracked(); err != nil {
		return false, err
	} else if len(untracked) != 0 {
		return false, errors.New("can't stash if there are untracked files")
	}
	if unstaged, err := g.Unstaged(); err != nil {
		return false, err
	} else if len(unstaged) == 0 {
		// No need to stash, there's no unstaged files.
		return false, nil
	}

	oldStash, _, _ := g.capture("rev-parse", "-q", "--verify", "refs/stash")
	if out, e, err := g.capture("stash", "save", "-q", "--keep-index"); e != 0 || err != nil {
		return false, fmt.Errorf("failed to stash: %s\n%s", err, out)
	}
	newStash, e, err := g.capture("rev-parse", "-q", "--verify", "refs/stash")
	if e != 0 || err != nil {
		return false, fmt.Errorf("failed to parse stash: %s\n%s", err, newStash)
	}
	return oldStash != newStash, err
}

func (g *git) Restore() error {
	if out, e, err := g.capture("reset", "--hard", "-q"); e != 0 || err != nil {
		return fmt.Errorf("git reset failed: %s\n%s", err, out)
	}
	if out, e, err := g.capture("stash", "apply", "--index", "-q"); e != 0 || err != nil {
		return fmt.Errorf("stash reapplication failed: %s\n%s", err, out)
	}
	if out, e, err := g.capture("stash", "drop", "-q"); e != 0 || err != nil {
		return fmt.Errorf("dropping temporary stash failed: %s\n%s", err, out)
	}
	return nil
}

func (g *git) capture(args ...string) (string, int, error) {
	return internal.CaptureWd(g.root, append([]string{"git"}, args...)...)
}

// getGitDir returns the .git directory path.
func getGitDir() (string, error) {
	gitDir, err := internal.CaptureAbs("git", "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("failed to find .git dir: %s", err)
	}
	return gitDir, err
}
