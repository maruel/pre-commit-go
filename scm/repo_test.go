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

func TestGetRepoGitSlow(t *testing.T) {
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
	r, err := getRepo(tmpDir)
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

	write(t, tmpDir, "file1", "hi\n")
	check(t, r, []string{"file1"}, []string{})

	run(t, tmpDir, nil, "add", "file1")
	check(t, r, []string{}, []string{})

	done, err := r.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, false, done)

	write(t, tmpDir, "file1", "hi\nhello\n")
	check(t, r, []string{}, []string{"file1"})

	done, err = r.Stash()
	ut.AssertEqual(t, errors.New("Can't stash until there's at least one commit"), err)
	ut.AssertEqual(t, false, done)

	// Author date is specified via --date but committer date is via environment
	// variable. Go figure.
	run(t, tmpDir, []string{"GIT_COMMITTER_DATE=2005-04-07T22:13:13 +0000"}, "commit", "-m", "yo", "--date", "2005-04-07T22:13:13 +0000")
	ut.AssertEqual(t, "master", r.Ref())
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))
	head := r.HEAD()
	if head != "56e6926b12ee571cfba4515214725b35a8571570" {
		t.Errorf("%s", strings.Join(os.Environ(), "\n"))
		t.Fatalf("%s", run(t, tmpDir, nil, "log", "-p", "--format=fuller"))
	}
	ut.AssertEqual(t, "master", r.Ref())

	done, err = r.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, true, done)
	ut.AssertEqual(t, "hi\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, nil, r.Restore())
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))

	ut.AssertEqual(t, errors.New("checkout failed:\nerror: pathspec 'invalid' did not match any file(s) known to git."), r.Checkout("invalid"))
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, "master", r.Ref())
	ut.AssertEqual(t, nil, r.Checkout(string(head)))
	ut.AssertEqual(t, "hi\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, "", r.Ref())
	ut.AssertEqual(t, head, r.HEAD())
	ut.AssertEqual(t, nil, r.Checkout("master"))
	ut.AssertEqual(t, "hi\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, "master", r.Ref())
	ut.AssertEqual(t, head, r.HEAD())
}

func TestGetRepoNoRepo(t *testing.T) {
	t.Parallel()
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := internal.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	r, err := GetRepo(tmpDir)
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
	r, err := getRepo(tmpDir)
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

func write(t *testing.T, tmpDir, name string, content string) {
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
