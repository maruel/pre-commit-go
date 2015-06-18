// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/ut"
)

func TestGetRepoGitSlowSuccess(t *testing.T) {
	// Make a repository and test behavior against it.
	t.Parallel()
	if isDrone() {
		t.Skipf("Give up on drone, it uses a weird go template which makes it not standard when using git init")
	}
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := internal.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	setup(t, tmpDir)
	r, err := getRepo(tmpDir, tmpDir)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, tmpDir, r.Root())
	p, err := r.HookPath()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, filepath.Join(tmpDir, ".git", "hooks"), p)
	ut.AssertEqual(t, GitInitialCommit, r.HEAD())
	err = r.Checkout(string(GitInitialCommit))
	ut.AssertEqual(t, errors.New("checkout failed:\nfatal: Cannot switch branch to a non-commit '4b825dc642cb6eb9a060e54bf8d69288fbee4904'"), err)

	ut.AssertEqual(t, []string{}, r.untracked())
	ut.AssertEqual(t, []string{}, r.unstaged())

	write(t, tmpDir, "src/foo/file1.go", "package foo\n")
	check(t, r, []string{"src/foo/file1.go"}, []string{})

	run(t, tmpDir, nil, "add", "src/foo/file1.go")
	check(t, r, []string{}, []string{})

	done, err := r.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, false, done)

	write(t, tmpDir, "src/foo/file1.go", "package foo\n// hello\n")
	check(t, r, []string{}, []string{"src/foo/file1.go"})

	done, err = r.Stash()
	ut.AssertEqual(t, errors.New("Can't stash until there's at least one commit"), err)
	ut.AssertEqual(t, false, done)

	deterministic_commit(t, tmpDir)
	ut.AssertEqual(t, "master", r.Ref())
	ut.AssertEqual(t, "package foo\n// hello\n", read(t, tmpDir, "src/foo/file1.go"))
	commitInitial := assertHEAD(t, r, "f4edb8ac30289340040451b6f8c20d17614a9ae7")
	ut.AssertEqual(t, "master", r.Ref())

	done, err = r.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, true, done)
	ut.AssertEqual(t, "package foo\n", read(t, tmpDir, "src/foo/file1.go"))
	ut.AssertEqual(t, nil, r.Restore())
	ut.AssertEqual(t, "package foo\n// hello\n", read(t, tmpDir, "src/foo/file1.go"))

	msg := "checkout failed:\nerror: pathspec 'invalid' did not match any file(s) known to git."
	ut.AssertEqual(t, errors.New(msg), r.Checkout("invalid"))
	ut.AssertEqual(t, "package foo\n// hello\n", read(t, tmpDir, "src/foo/file1.go"))
	ut.AssertEqual(t, "master", r.Ref())
	ut.AssertEqual(t, nil, r.Checkout(string(commitInitial)))
	ut.AssertEqual(t, "package foo\n", read(t, tmpDir, "src/foo/file1.go"))
	ut.AssertEqual(t, "", r.Ref())
	ut.AssertEqual(t, commitInitial, r.HEAD())
	ut.AssertEqual(t, nil, r.Checkout("master"))
	ut.AssertEqual(t, "package foo\n", read(t, tmpDir, "src/foo/file1.go"))
	ut.AssertEqual(t, "master", r.Ref())
	ut.AssertEqual(t, commitInitial, r.HEAD())

	upstream, err := r.Upstream()
	ut.AssertEqual(t, Commit(""), upstream)
	ut.AssertEqual(t, errors.New("no upstream"), err)

	c, err := r.Between(commitInitial, GitInitialCommit, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string{"src/foo/file1.go"}, c.Changed().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/file1.go"}, c.Indirect().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/file1.go"}, c.All().GoFiles())

	c, err = r.Between(Current, GitInitialCommit, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string{"src/foo/file1.go"}, c.Changed().GoFiles())

	c, err = r.Between(Current, GitInitialCommit, []string{"f*"})
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, c)

	c, err = r.Between(Current, commitInitial, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, c)

	c, err = r.Between(commitInitial, Current, nil)
	ut.AssertEqual(t, errors.New("can't use Current as old commit"), err)
	ut.AssertEqual(t, nil, c)

	c, err = r.Between(commitInitial, Commit("foo"), nil)
	ut.AssertEqual(t, errors.New("invalid old commit"), err)
	ut.AssertEqual(t, nil, c)

	// Add a file then remove it. Make sure the file doesn't show up.
	check(t, r, []string{}, []string{})
	write(t, tmpDir, "src/foo/deleted/deleted.go", "package deleted\n")
	run(t, tmpDir, nil, "add", "src/foo/deleted/deleted.go")
	deterministic_commit(t, tmpDir)
	commitWithDeleted := assertHEAD(t, r, "c9b5f312ec8eefb58beeaf8c3684bb832fdefef7")
	c, err = r.Between(commitWithDeleted, GitInitialCommit, nil)
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.Changed().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.Indirect().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.All().GoFiles())
	c, err = r.Between(commitWithDeleted, commitInitial, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go"}, c.Changed().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go"}, c.Indirect().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.All().GoFiles())
	ut.AssertEqual(t, nil, err)

	// Do the delete.
	run(t, tmpDir, nil, "rm", "src/foo/deleted/deleted.go")
	deterministic_commit(t, tmpDir)
	commitAfterDelete := assertHEAD(t, r, "8aacb7c27c4d012c56bd861d2a8bc4da8ea7ee73")
	c, err = r.Between(commitAfterDelete, GitInitialCommit, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string{"src/foo/file1.go"}, c.Changed().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/file1.go"}, c.Indirect().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/file1.go"}, c.All().GoFiles())
	c, err = r.Between(commitAfterDelete, commitWithDeleted, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, c)
	c, err = r.Between(commitWithDeleted, GitInitialCommit, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.Changed().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.Indirect().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.All().GoFiles())
	c, err = r.Between(commitWithDeleted, commitInitial, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go"}, c.Changed().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go"}, c.Indirect().GoFiles())
	ut.AssertEqual(t, []string{"src/foo/deleted/deleted.go", "src/foo/file1.go"}, c.All().GoFiles())
	c, err = r.Between(Current, commitInitial, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, c)
	c, err = r.Between(Current, commitAfterDelete, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, c)
	c, err = r.Between(Current, commitWithDeleted, nil)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, c)
}

func TestGetRepoNoRepo(t *testing.T) {
	t.Parallel()
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := internal.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	r, err := GetRepo(tmpDir, "")
	ut.AssertEqual(t, errors.New("failed to find git checkout root"), err)
	ut.AssertEqual(t, nil, r)
}

func TestGetRepoGitSlowFailures(t *testing.T) {
	t.Parallel()
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := internal.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	setup(t, tmpDir)
	r, err := getRepo(tmpDir, tmpDir)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, tmpDir, r.Root())
	// Remove the .git directory after calling GetRepo().
	ut.AssertEqual(t, nil, internal.RemoveAll(filepath.Join(tmpDir, ".git")))

	p, err := r.HookPath()
	ut.AssertEqual(t, errors.New("failed to find .git dir: failed to find .git dir: failed to run \"git rev-parse --git-dir\""), err)
	ut.AssertEqual(t, "", p)

	ut.AssertEqual(t, []string(nil), r.untracked())
	ut.AssertEqual(t, []string(nil), r.unstaged())

	ut.AssertEqual(t, GitInitialCommit, r.HEAD())
	ut.AssertEqual(t, "", r.Ref())

	done, err := r.Stash()
	ut.AssertEqual(t, errors.New("failed to get list of untracked files"), err)
	ut.AssertEqual(t, false, done)
	errStr := r.Restore().Error()
	if errStr != "git reset failed:\nfatal: Not a git repository: '.git'" && errStr != "git reset failed:\nfatal: Not a git repository (or any of the parent directories): .git" {
		t.Fatalf("Unexpected error: %s", errStr)
	}

	errStr = r.Checkout(string(GitInitialCommit)).Error()
	if errStr != "checkout failed:\nfatal: Not a git repository: '.git'" && errStr != "checkout failed:\nfatal: Not a git repository (or any of the parent directories): .git" {
		t.Fatalf("Unexpected error: %s", errStr)
	}
}

// Private stuff.

func setup(t *testing.T, tmpDir string) {
	_, code, err := internal.Capture(tmpDir, nil, "git", "init")
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)
	run(t, tmpDir, nil, "config", "user.email", "nobody@localhost")
	run(t, tmpDir, nil, "config", "user.name", "nobody")
}

func check(t *testing.T, r repo, untracked []string, unstaged []string) {
	ut.AssertEqual(t, untracked, r.untracked())
	ut.AssertEqual(t, unstaged, r.unstaged())
}

func run(t *testing.T, tmpDir string, env []string, args ...string) string {
	internal := &git{root: tmpDir}
	out, code, err := internal.capture(env, args...)
	ut.AssertEqualf(t, 0, code, "%s", out)
	ut.AssertEqual(t, nil, err)
	return out
}

// deterministic_commit generates a commit that has always the same hash.
//
// Author date is specified via --date but committer date is via environment
// variable. Go figure.
func deterministic_commit(t *testing.T, tmpDir string) {
	run(t, tmpDir, []string{"GIT_COMMITTER_DATE=2005-04-07T22:13:13 +0000"}, "commit", "-m", "yo", "--date", "2005-04-07T22:13:13 +0000")
}

func assertHEAD(t *testing.T, r ReadOnlyRepo, expected Commit) Commit {
	if head := r.HEAD(); head != expected {
		t.Logf("%s", strings.Join(os.Environ(), "\n"))
		t.Logf("%s", run(t, r.Root(), nil, "log", "-p", "--format=fuller"))
		ut.AssertEqual(t, expected, head)
	}
	return expected
}

func write(t *testing.T, tmpDir, name string, content string) {
	if d := filepath.Dir(name); d != "" {
		ut.AssertEqual(t, nil, os.MkdirAll(filepath.Join(tmpDir, d), 0700))
	}
	ut.AssertEqual(t, nil, ioutil.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0600))
}

func read(t *testing.T, tmpDir, name string) string {
	content, err := ioutil.ReadFile(filepath.Join(tmpDir, name))
	ut.AssertEqual(t, nil, err)
	return string(content)
}

// isDrone returns true if running under https://drone.io.
//
// See http://docs.drone.io/env.html
func isDrone() bool {
	return os.Getenv("DRONE") == "true"
}
