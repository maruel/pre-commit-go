// Copyright 2016 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package scm implements repository management specific for Go projects.
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
	// Initial is the root invisible commit.
	Initial Commit = "<initial>"
	// Head is the reference to the current checkout as referenced by what is
	// checked in.
	Head Commit = "<head>"
	// Current is a meta-reference to the current tree as on the file system.
	Current Commit = "<current>"
	// Upstream is the commit on the remote repository against with the current
	// branch is based on.
	Upstream Commit = "<upstream>"
	// Invalid is an invalid commit reference.
	Invalid Commit = "<invalid>"
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
	// Ref returns the branch name referencing to commit c. If there is no branch
	// name, "" is returned.
	Ref(c Commit) string
	// Eval returns the commit hash by evaluating refish. Returns Invalid in case
	// of failure.
	Eval(refish string) Commit
	// Between returns a change with files touched between from and to in it.
	// If recent is Current, it diffs against the current tree, independent of
	// what is versioned.
	//
	// To get files in the staging area, use (Current, Head).
	//
	// Untracked files are always excluded.
	//
	// Files with untracked change will be included if recent == Current. To
	// exclude untracked changes to tracked files, use Stash() first or specify
	// Head for recent.
	//
	// To get the list of all files in the tree and the index, use
	// Between(Current, Initial, ...).
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
	Checkout(refish string) error
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
				log.Printf("%s: ignored due to %q", p, ignorePattern)
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
	// staged returns the list of files in the index.
	staged() []string
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

type gitCommit Commit

const (
	gitInitial  gitCommit = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
	gitHead     gitCommit = "HEAD"
	gitCurrent  gitCommit = "<current>"
	gitUpstream gitCommit = "@{upstream}"
	gitInvalid  gitCommit = "<invalid>"
)

func toGitCommit(c Commit) gitCommit {
	switch c {
	case Initial:
		return gitInitial
	case Head:
		return gitHead
	case Current:
		return gitCurrent
	case Upstream:
		return gitUpstream
	case Invalid, "":
		return gitInvalid
	default:
		return gitCommit(c)
	}
}

type git struct {
	root   string
	gopath string

	lock   sync.Mutex
	gitDir string
}

// ReadOnlyRepo interface.

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

func (g *git) Ref(c Commit) string {
	gc := toGitCommit(c)
	if gc == gitInvalid {
		return string(Invalid)
	}
	// Semantically, Current == Head for the Ref.
	if gc == gitCurrent {
		gc = gitHead
	}
	out, code, _ := g.capture("symbolic-ref", "--short", string(gc))
	if code == 0 {
		return out
	}
	log.Println(out)
	return ""
}

func (g *git) Eval(refish string) Commit {
	// Look for meta-commit. Branch names will be passing fine, unless there's a
	// branch named "<invalid>".
	c := toGitCommit(Commit(refish))
	if c == gitCurrent {
		c = gitHead
	}
	if c == gitInitial {
		// Shortcut.
		return Commit(gitInitial)
	}
	if c == gitInvalid {
		return Invalid
	}
	out, code, _ := g.capture("log", "-1", "--format=%H", string(c))
	if code == 0 {
		return Commit(out)
	}
	if c == gitHead {
		// It's because there hasn't been a commit yet.
		return Commit(gitInitial)
	}
	log.Println(out)
	return Invalid
}

func (g *git) Between(recent, old Commit, ignorePatterns IgnorePatterns) (Change, error) {
	log.Printf("Between(%q, %q, %s)", recent, old, ignorePatterns)
	grecent := toGitCommit(recent)
	if grecent == gitInvalid {
		return nil, errors.New("invalid recent commit")
	}
	if grecent != gitCurrent && !g.isValid(grecent) {
		return nil, errors.New("invalid recent commit")
	}
	gold := toGitCommit(old)
	if gold == gitInvalid {
		return nil, errors.New("invalid old commit")
	}
	if gold == gitCurrent {
		return nil, errors.New("can't use Current as old commit")
	}
	if gold != gitUpstream && gold != gitHead && !g.isValid(gold) {
		return nil, errors.New("invalid old commit")
	}

	// Gather list of all files concurrently.
	allFilesCh := make(chan []string)
	var allFiles []string

	// Gather list of changed files.
	var files []string
	filesEqualsAllFiles := false
	if grecent == gitCurrent {
		// Current is special cased, as it has to look at the checked out files.
		go func() {
			allFilesCh <- g.captureList(ignorePatterns, "ls-files", "-z")
		}()
		if gold == gitInitial {
			// Fast path: diff against initial commit.
			allFiles = <-allFilesCh
			files = allFiles
			filesEqualsAllFiles = true
		} else {
			// Gather list of unstaged file plus diff.
			unstagedCh := make(chan []string)
			go func() {
				unstagedCh <- g.unstaged()
			}()
			stagedCh := make(chan []string)
			go func() {
				stagedCh <- g.staged()
			}()

			// Need to remove duplicates.
			// TODO(maruel): Use github.com/xtgo/set
			filesSet := map[string]struct{}{}
			for _, f := range g.captureList(ignorePatterns, "diff-tree", "--no-commit-id", "--name-only", "-z", "-r", "--diff-filter=ACMRT", "--no-renames", "--no-ext-diff", string(gold), string(gitHead)) {
				filesSet[f] = struct{}{}
			}
			for _, f := range <-unstagedCh {
				filesSet[f] = struct{}{}
			}
			for _, f := range <-stagedCh {
				filesSet[f] = struct{}{}
			}
			files = make([]string, 0, len(filesSet))
			for f := range filesSet {
				files = append(files, f)
			}
			allFiles = <-allFilesCh
		}
	} else {
		// Not using Current, so only use the index.
		go func() {
			allFilesCh <- g.captureList(ignorePatterns, "ls-files", "-z", "--with-tree="+string(grecent))
		}()
		files = g.captureList(ignorePatterns, "diff-tree", "--no-commit-id", "--name-only", "-z", "-r", "--diff-filter=ACMRT", "--no-renames", "--no-ext-diff", string(gold), string(grecent))
		allFiles = <-allFilesCh
	}
	if len(files) == 0 {
		return nil, nil
	}

	// Sort concurrently.
	var wg sync.WaitGroup
	if !filesEqualsAllFiles {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sort.Strings(files)
		}()
	}
	sort.Strings(allFiles)
	wg.Wait()

	return newChange(g, files, allFiles, ignorePatterns), nil
}

func (g *git) GOPATH() string {
	return g.gopath
}

// Repo interface.

func (g *git) Stash() (bool, error) {
	// Ensure everything is either tracked or ignored. This is because git stash
	// doesn't stash untracked files.
	// The 2 checks are run in parallel with the first stashing command.
	errUntrackedCh := make(chan error)
	go func() {
		if untracked := g.untracked(); untracked == nil {
			errUntrackedCh <- errors.New("failed to get list of untracked files")
		} else if len(untracked) != 0 {
			errUntrackedCh <- fmt.Errorf("can't stash if there are untracked files: %q", untracked)
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
		o, _, _ := g.capture("rev-parse", "-q", "--verify", "refs/stash")
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

	if out, e, err := g.capture("stash", "save", "-q", "--keep-index"); e != 0 || err != nil {
		if gitCommit(g.Eval(string(gitHead))) == gitInitial {
			return false, errors.New("Can't stash until there's at least one commit")
		}
		return false, fmt.Errorf("failed to stash:\n%s", out)
	}
	newStash, e, err := g.capture("rev-parse", "-q", "--verify", "refs/stash")
	if e != 0 || err != nil {
		return false, fmt.Errorf("failed to parse stash: %s\n%s", err, newStash)
	}
	return oldStash != newStash, err
}

func (g *git) Restore() error {
	if out, e, err := g.capture("reset", "--hard", "-q"); e != 0 || err != nil {
		return fmt.Errorf("git reset failed:\n%s", out)
	}
	if out, e, err := g.capture("stash", "apply", "--index", "-q"); e != 0 || err != nil {
		return fmt.Errorf("stash reapplication failed:\n%s", out)
	}
	if out, e, err := g.capture("stash", "drop", "-q"); e != 0 || err != nil {
		return fmt.Errorf("dropping temporary stash failed:\n%s", out)
	}
	return nil
}

func (g *git) Checkout(refish string) error {
	c := toGitCommit(Commit(refish))
	if c == gitInvalid {
		return errors.New("invalid commit")
	}
	if out, e, err := g.capture("checkout", "-f", "-q", string(c)); e != 0 || err != nil {
		return fmt.Errorf("checkout failed:\n%s", out)
	}
	return nil
}

func (g *git) untracked() []string {
	return g.captureList(nil, "ls-files", "--others", "--exclude-standard", "-z")
}

func (g *git) unstaged() []string {
	return g.captureList(nil, "diff", "--name-only", "--no-color", "--no-ext-diff", "-z")
}

func (g *git) staged() []string {
	return g.captureList(nil, "diff", "--name-only", "--no-color", "--no-ext-diff", "--cached", "--diff-filter=ACMRT", "-z")
}

func (g *git) capture(args ...string) (string, int, error) {
	return g.captureEnv(nil, args...)
}

func (g *git) captureEnv(env []string, args ...string) (string, int, error) {
	out, code, err := internal.Capture(g.root, env, append([]string{"git"}, args...)...)
	return strings.TrimRight(out, "\n\r"), code, err
}

// captureList assumes the -z argument is used. Returns nil in case of error.
//
// It strips any file in ignorePatterns glob that applies to any path component.
func (g *git) captureList(ignorePatterns IgnorePatterns, args ...string) []string {
	// TOOD(maruel): stream stdout instead of taking the whole output at once. It
	// may only have an effect on larger repositories and that's not guaranteed.
	// For example, the output of "git ls-files -z" on the chromium tree with 86k
	// files is 4.5Mib and takes ~110ms to run. Revisit later when this becomes a
	// bottleneck.
	out, code, err := g.capture(args...)
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

func (g *git) isValid(c gitCommit) bool {
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
