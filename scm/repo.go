// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package scm implements repository management.
package scm

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/maruel/pre-commit-go/internal"
)

// GitInitialCommit is the root invisible commit.
const GitInitialCommit = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// ReadOnlyRepo represents a source control managemed checkout.
//
// ReadOnlyRepo exposes no function that would modify the state of the checkout.
type ReadOnlyRepo interface {
	// Root returns the root directory of this repository.
	Root() string
	// HookPath returns the directory containing the commit and push hooks.
	HookPath() (string, error)
	// HEAD returns the HEAD commit hash.
	HEAD() string
	// Ref returns the HEAD branch name.
	Ref() string
	// Untracked returns the list of untracked files.
	Untracked() ([]string, error)
	// Unstaged returns the list with changes not in the staging index.
	Unstaged() ([]string, error)
	// All returns a change with everything in it.
	All() Change
}

// Repo represents a source control managed checkout.
//
// It is possible to modify this repository with the functions exposed by this
// interface.
type Repo interface {
	ReadOnlyRepo
	// Stash stashes the content that is not in the index.
	Stash() (bool, error)
	// Stash restores the stash generated from Stash.
	Restore() error
	// Checkout checks out a commit.
	Checkout(commit string) error
}

// GetRepo returns a valid Repo if one is found.
func GetRepo(wd string) (Repo, error) {
	root, err := captureAbs(wd, "git", "rev-parse", "--show-cdup")
	if err == nil {
		return &git{root: root}, nil
	}
	// TODO: Add your favorite SCM.
	return nil, fmt.Errorf("failed to find git checkout root")
}

// Private details.

var reCommit = regexp.MustCompile("^[0-9a-f]{40}$")

type git struct {
	root   string
	gitDir string
}

func (g *git) Root() string {
	return g.root
}

func (g *git) HookPath() (string, error) {
	if g.gitDir == "" {
		var err error
		g.gitDir, err = getGitDir(g.root)
		if err != nil {
			return "", fmt.Errorf("failed to find .git dir: %s", err)
		}
	}
	return filepath.Join(g.gitDir, "hooks"), nil
}

func (g *git) HEAD() string {
	if out, code, _ := g.capture(nil, "rev-parse", "--verify", "HEAD"); code == 0 {
		return out
	}
	return GitInitialCommit
}

func (g *git) Ref() string {
	if out, code, _ := g.capture(nil, "symbolic-ref", "--short", "HEAD"); code == 0 {
		return out
	}
	return ""
}

func (g *git) Untracked() ([]string, error) {
	out, code, err := g.capture(nil, "ls-files", "--others", "--exclude-standard")
	if code != 0 || err != nil {
		return nil, errors.New("failed to retrieve untracked files")
	}
	if len(out) != 0 {
		return strings.Split(out, "\n"), err
	}
	return nil, err
}

func (g *git) Unstaged() ([]string, error) {
	out, code, err := g.capture(nil, "diff", "--name-only", "--no-color", "--no-ext-diff")
	if code != 0 || err != nil {
		return nil, errors.New("failed to retrieve unstaged files")
	}
	if len(out) != 0 {
		return strings.Split(out, "\n"), err
	}
	return nil, err
}

func (g *git) All() Change {
	return &change{repo: g}
}

func (g *git) Stash() (bool, error) {
	// Ensure everything is either tracked or ignored. This is because git stash
	// doesn't stash untracked files.
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
	oldStash, _, _ := g.capture(nil, "rev-parse", "-q", "--verify", "refs/stash")
	if out, e, err := g.capture(nil, "stash", "save", "-q", "--keep-index"); e != 0 || err != nil {
		if g.HEAD() == GitInitialCommit {
			return false, errors.New("Can't stash until there's at least one commit")
		}
		return false, fmt.Errorf("failed to stash:\n%s", out)
	}
	newStash, e, err := g.capture(nil, "rev-parse", "-q", "--verify", "refs/stash")
	if e != 0 || err != nil {
		return false, fmt.Errorf("failed to parse stash: %s\n%s", err, newStash)
	}
	return oldStash != newStash, err
}

func (g *git) Restore() error {
	if out, e, err := g.capture(nil, "reset", "--hard", "-q"); e != 0 || err != nil {
		return fmt.Errorf("git reset failed:\n%s", out)
	}
	if out, e, err := g.capture(nil, "stash", "apply", "--index", "-q"); e != 0 || err != nil {
		return fmt.Errorf("stash reapplication failed:\n%s", out)
	}
	if out, e, err := g.capture(nil, "stash", "drop", "-q"); e != 0 || err != nil {
		return fmt.Errorf("dropping temporary stash failed:\n%s", out)
	}
	return nil
}

func (g *git) Checkout(commit string) error {
	if !reCommit.MatchString(commit) {
		return errors.New("only commit hash is accepted")
	}
	if out, e, err := g.capture(nil, "checkout", "-f", "-q", commit); e != 0 || err != nil {
		return fmt.Errorf("checkout failed:\n%s", out)
	}
	return nil
}

func (g *git) capture(env []string, args ...string) (string, int, error) {
	out, code, err := internal.Capture(g.root, env, append([]string{"git"}, args...)...)
	return strings.TrimRight(out, "\n\r"), code, err
}

// getGitDir returns the .git directory path.
func getGitDir(wd string) (string, error) {
	gitDir, err := captureAbs(wd, "git", "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("failed to find .git dir: %s", err)
	}
	return gitDir, err
}

// captureAbs returns an absolute path of whatever a git command returned.
func captureAbs(wd string, args ...string) (string, error) {
	out, code, _ := internal.Capture(wd, nil, args...)
	if code != 0 {
		return "", fmt.Errorf("failed to run \"%s\"", strings.Join(args, " "))
	}
	out = strings.TrimSpace(out)
	if !filepath.IsAbs(out) {
		out = filepath.Clean(filepath.Join(wd, out))
	}
	return out, nil
}
