// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package scm implements repository management.
package scm

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/maruel/pre-commit-go/internal"
)

// Commit represents a commit reference, normally a digest.
type Commit string

const (
	// GitInitialCommit is the root invisible commit.
	// TODO(maruel): When someone want to add mercurial support, refactor this to
	// create a pseudo constant named InitialCommit.
	GitInitialCommit Commit = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	// Current is a meta-reference to the current tree.
	Current Commit = ""
)

// ReadOnlyRepo represents a source control managemed checkout.
//
// ReadOnlyRepo exposes no function that would modify the state of the checkout.
//
// The implementation of this interface must be thread safe.
type ReadOnlyRepo interface {
	// Root returns the root directory of this repository.
	Root() string
	// Scmdir returns the directory containing the source control specific files,
	// e.g. it is ".git" by default for git repositories. It can be different
	// when GIT_DIR is specified or in the case of git submodules.
	ScmDir() (string, error)
	// HookPath returns the directory containing the commit and push hooks.
	HookPath() (string, error)
	// HEAD returns the HEAD commit hash.
	HEAD() Commit
	// Ref returns the HEAD branch name if any. If a remote branch is checked
	// out, "" is returned.
	Ref() string
	// Upstream returns the upstream commit.
	Upstream() (Commit, error)

	// Between returns a change with files touched between from and to in it.
	// If recent is Current, it diffs against the current tree, independent of
	// what is versioned.
	//
	// To get files in the staging area, use (Current, HEAD()).
	//
	// Untracked files are always excluded.
	//
	// Files with untracked change will be included if recent == Current. To
	// exclude untracked changes to tracked files, use Stash() first or specify
	// recent=HEAD().
	//
	// To get the list of all files in the tree and the index, use
	// Between(Current, GitInitialCommit, ...).
	//
	// Returns nil and no error if there's no file difference.
	Between(recent, old Commit, ignorePatterns IgnorePatterns) (Change, error)
	// GOPATH returns the GOPATH. Mostly used in tests.
	GOPATH() string
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
	// Checkout checks out a commit or a branch.
	Checkout(ref string) error
}

// GetRepo returns a valid Repo if one is found.
func GetRepo(wd, gopath string) (Repo, error) {
	return getRepo(wd, gopath)
}

// IgnorePatterns is a list of glob that when matching, means the file should
// be ignored.
type IgnorePatterns []string

// Match returns true when the file should be ignored.
func (i *IgnorePatterns) Match(p string) bool {
	chunks := strings.Split(p, pathSeparator)
	for _, ignorePattern := range *i {
		for _, chunk := range chunks {
			if matched, err := filepath.Match(ignorePattern, chunk); matched {
				return true
			} else if err != nil {
				log.Printf("bad pattern %q", ignorePattern)
			}
		}
	}
	return false
}

func (i *IgnorePatterns) String() string {
	return fmt.Sprintf("%s", *i)
}

// Set implements flag.Value.
func (i *IgnorePatterns) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// Private details.

var reCommit = regexp.MustCompile("^[0-9a-f]{40}$")

type repo interface {
	Repo
	// untracked returns the list of untracked files.
	untracked() []string
	// unstaged returns the list with changes not in the staging index.
	unstaged() []string
}

func getRepo(wd, gopath string) (repo, error) {
	root, err := captureAbs(wd, "git", "rev-parse", "--show-cdup")
	if err == nil {
		if gopath == "" {
			gopath = os.Getenv("GOPATH")
		}
		return &git{root: root, gopath: gopath}, nil
	}
	// TODO: Add your favorite SCM.
	return nil, fmt.Errorf("failed to find git checkout root")
}

type git struct {
	root   string
	gopath string

	lock   sync.Mutex
	gitDir string
}

func (g *git) Root() string {
	return g.root
}

func (g *git) ScmDir() (string, error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.gitDir == "" {
		var err error
		g.gitDir, err = getGitDir(g.root)
		if err != nil {
			return "", fmt.Errorf("failed to find .git dir: %s", err)
		}
	}
	return g.gitDir, nil
}

func (g *git) HookPath() (string, error) {
	d, err := g.ScmDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "hooks"), nil
}

func (g *git) HEAD() Commit {
	if out, code, _ := g.capture(nil, "rev-parse", "--verify", "HEAD"); code == 0 {
		return Commit(out)
	}
	return GitInitialCommit
}

func (g *git) Ref() string {
	if out, code, _ := g.capture(nil, "symbolic-ref", "--short", "HEAD"); code == 0 {
		return out
	}
	return ""
}

func (g *git) Upstream() (Commit, error) {
	if out, code, _ := g.capture(nil, "log", "-1", "--format=%H", "@{upstream}"); code == 0 {
		return Commit(out), nil
	}
	return "", errors.New("no upstream")
}

func (g *git) untracked() []string {
	return g.captureList(nil, nil, "ls-files", "--others", "--exclude-standard", "-z")
}

func (g *git) unstaged() []string {
	return g.captureList(nil, nil, "diff", "--name-only", "--no-color", "--no-ext-diff", "-z")
}

func (g *git) Between(recent, old Commit, ignorePatterns IgnorePatterns) (Change, error) {
	log.Printf("Between(%q, %q, %s)", recent, old, ignorePatterns)
	if old == Current {
		return nil, errors.New("can't use Current as old commit")
	}
	if !g.isValid(old) {
		return nil, errors.New("invalid old commit")
	}

	// Gather list of all files concurrently.
	allFilesCh := make(chan []string)
	var allFiles []string

	// Gather list of changes files.
	var files []string
	if recent == Current {
		go func() {
			allFilesCh <- g.captureList(nil, ignorePatterns, "ls-files", "-z")
		}()
		if old == GitInitialCommit {
			// Diff against initial commit.
			allFiles = <-allFilesCh
			files = allFiles
		} else {
			// Gather list of unstaged file plus diff.
			unstagedCh := make(chan []string)
			go func() {
				// TODO(maruel): A a test case, not 100% sure it's needed.
				unstagedCh <- g.unstaged()
			}()

			// Need to remove duplicates.
			filesSet := map[string]bool{}
			for _, f := range g.captureList(nil, ignorePatterns, "diff-tree", "--no-commit-id", "--name-only", "-z", "-r", "--diff-filter=ACMRT", "--no-renames", "--no-ext-diff", string(old), "HEAD") {
				filesSet[f] = true
			}
			for _, f := range <-unstagedCh {
				filesSet[f] = true
			}
			files = make([]string, 0, len(filesSet))
			for f := range filesSet {
				files = append(files, f)
			}
			allFiles = <-allFilesCh
		}
	} else {
		go func() {
			allFilesCh <- g.captureList(nil, ignorePatterns, "ls-files", "-z", "--with-tree="+string(recent))
		}()
		if !g.isValid(recent) {
			return nil, errors.New("invalid old commit")
		}
		files = g.captureList(nil, ignorePatterns, "diff-tree", "--no-commit-id", "--name-only", "-z", "-r", "--diff-filter=ACMRT", "--no-renames", "--no-ext-diff", string(old), string(recent))
		allFiles = <-allFilesCh
	}
	if len(files) == 0 {
		return nil, nil
	}

	// Sort concurrently.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sort.Strings(files)
	}()
	sort.Strings(allFiles)
	wg.Wait()

	return newChange(g, files, allFiles, ignorePatterns), nil
}

func (g *git) GOPATH() string {
	return g.gopath
}

func (g *git) Stash() (bool, error) {
	// Ensure everything is either tracked or ignored. This is because git stash
	// doesn't stash untracked files.
	// The 2 checks are run in parallel with the first stashing command.
	errUntrackedCh := make(chan error)
	go func() {
		if untracked := g.untracked(); untracked == nil {
			errUntrackedCh <- errors.New("failed to get list of untracked files")
		} else if len(untracked) != 0 {
			errUntrackedCh <- errors.New("can't stash if there are untracked files")
		} else {
			errUntrackedCh <- nil
		}
	}()

	errUnstagedCh := make(chan error)
	ignore := errors.New("ignore")
	go func() {
		if unstaged := g.unstaged(); unstaged == nil {
			errUnstagedCh <- errors.New("failed to get list of unstaged files")
		} else if len(unstaged) == 0 {
			// No need to stash, there's no unstaged files.
			errUnstagedCh <- ignore
		} else {
			errUnstagedCh <- nil
		}
	}()

	oldStashCh := make(chan string)
	go func() {
		o, _, _ := g.capture(nil, "rev-parse", "-q", "--verify", "refs/stash")
		oldStashCh <- o
	}()

	// Error handling of concurrent processes.
	if err := <-errUntrackedCh; err != nil {
		return false, err
	}
	if err := <-errUnstagedCh; err == ignore {
		// No need to stash, there's no unstaged files.
		return false, nil
	} else if err != nil {
		return false, err
	}
	oldStash := <-oldStashCh

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

func (g *git) Checkout(ref string) error {
	if out, e, err := g.capture(nil, "checkout", "-f", "-q", ref); e != 0 || err != nil {
		return fmt.Errorf("checkout failed:\n%s", out)
	}
	return nil
}

func (g *git) capture(env []string, args ...string) (string, int, error) {
	out, code, err := internal.Capture(g.root, env, append([]string{"git"}, args...)...)
	return strings.TrimRight(out, "\n\r"), code, err
}

// captureList assumes the -z argument is used. Returns nil in case of error.
//
// It strips any file in ignorePatterns glob that applies to any path component.
func (g *git) captureList(env []string, ignorePatterns IgnorePatterns, args ...string) []string {
	// TOOD(maruel): stream stdout instead of taking the whole output at once. It
	// may only have an effect on larger repositories and that's not guaranteed.
	// For example, the output of "git ls-files -z" on the chromium tree with 86k
	// files is 4.5Mib and takes ~110ms to run. Revisit later when this becomes a
	// bottleneck.
	out, code, err := g.capture(env, args...)
	if code != 0 || err != nil {
		return nil
	}
	// Reduce initial memory allocation churn.
	list := make([]string, 0, 128)
	for {
		i := strings.IndexByte(out, 0)
		if i <= 0 {
			break
		}
		s := out[:i]
		if !ignorePatterns.Match(s) {
			list = append(list, s)
		}
		out = out[i+1:]
	}
	return list
}

func (g *git) isValid(c Commit) bool {
	return reCommit.MatchString(string(c))
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
