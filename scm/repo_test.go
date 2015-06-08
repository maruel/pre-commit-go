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

func init() {
	// Remove any GIT_ function, since it can change git behavior significantly
	// during the test that it can break them. For example GIT_DIR,
	// GIT_INDEX_FILE, GIT_PREFIX, GIT_AUTHOR_NAME, GIT_EDITOR are set when the
	// test is run under a git hook like pre-commit.
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "GIT_") {
			items := strings.SplitN(item, "=", 2)
			_ = os.Unsetenv(items[0])
		}
	}
}

func TestGetRepoGitSlow(t *testing.T) {
	// Make a repository and test behavior against it.
	t.Parallel()
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := internal.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	setup(t, tmpDir)
	repo, err := GetRepo(tmpDir)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, tmpDir, repo.Root())
	p, err := repo.HookPath()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, filepath.Join(tmpDir, ".git", "hooks"), p)
	ut.AssertEqual(t, GitInitialCommit, repo.HEAD())
	err = repo.Checkout(GitInitialCommit)
	base := "checkout failed:\nfatal: Cannot switch branch to a non-commit"
	if os.Getenv("DRONE") == "true" {
		// #thanksdrone
		ut.AssertEqual(t, errors.New(base+"."), err)
	} else {
		ut.AssertEqual(t, errors.New(base+" '4b825dc642cb6eb9a060e54bf8d69288fbee4904'"), err)
	}

	untracked, err := repo.Untracked()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string(nil), untracked)

	unstaged, err := repo.Unstaged()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, []string(nil), unstaged)

	write(t, tmpDir, "file1", "hi\n")
	check(t, repo, []string{"file1"}, nil)

	run(t, tmpDir, nil, "add", "file1")
	check(t, repo, nil, nil)

	done, err := repo.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, false, done)

	write(t, tmpDir, "file1", "hi\nhello\n")
	check(t, repo, nil, []string{"file1"})

	done, err = repo.Stash()
	ut.AssertEqual(t, errors.New("Can't stash until there's at least one commit"), err)
	ut.AssertEqual(t, false, done)

	// Author date is specified via --date but committer date is via environment
	// variable. Go figure.
	run(t, tmpDir, []string{"GIT_COMMITTER_DATE=2005-04-07T22:13:13 +0000"}, "commit", "-m", "yo", "--date", "2005-04-07T22:13:13 +0000")
	ut.AssertEqual(t, "master", repo.Ref())
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))
	head := repo.HEAD()
	if head != "56e6926b12ee571cfba4515214725b35a8571570" {
		t.Errorf("%s", strings.Join(os.Environ(), "\n"))
		t.Fatalf("%s", run(t, tmpDir, nil, "log", "-p", "--format=fuller"))
	}
	ut.AssertEqual(t, "master", repo.Ref())

	done, err = repo.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, true, done)
	ut.AssertEqual(t, "hi\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, nil, repo.Restore())
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))

	ut.AssertEqual(t, errors.New("only commit hash is accepted"), repo.Checkout("invalid"))
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, "master", repo.Ref())
	ut.AssertEqual(t, nil, repo.Checkout(head))
	ut.AssertEqual(t, "hi\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, "", repo.Ref())
}

func TestGetRepoNoRepo(t *testing.T) {
	t.Parallel()
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := internal.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	repo, err := GetRepo(tmpDir)
	ut.AssertEqual(t, errors.New("failed to find git checkout root"), err)
	ut.AssertEqual(t, nil, repo)
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
	repo, err := GetRepo(tmpDir)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, tmpDir, repo.Root())
	// Remove the .git directory after calling GetRepo().
	ut.AssertEqual(t, nil, internal.RemoveAll(filepath.Join(tmpDir, ".git")))

	p, err := repo.HookPath()
	ut.AssertEqual(t, errors.New("failed to find .git dir: failed to find .git dir: failed to run \"git rev-parse --git-dir\""), err)
	ut.AssertEqual(t, "", p)

	untracked, err := repo.Untracked()
	ut.AssertEqual(t, errors.New("failed to retrieve untracked files"), err)
	ut.AssertEqual(t, []string(nil), untracked)

	ut.AssertEqual(t, GitInitialCommit, repo.HEAD())
	ut.AssertEqual(t, "", repo.Ref())

	unstaged, err := repo.Unstaged()
	ut.AssertEqual(t, errors.New("failed to retrieve unstaged files"), err)
	ut.AssertEqual(t, []string(nil), unstaged)

	done, err := repo.Stash()
	ut.AssertEqual(t, errors.New("failed to retrieve untracked files"), err)
	ut.AssertEqual(t, false, done)
	errStr := repo.Restore().Error()
	if errStr != "git reset failed:\nfatal: Not a git repository: '.git'" && errStr != "git reset failed:\nfatal: Not a git repository (or any of the parent directories): .git" {
		t.Fatalf("Unexpected error: %s", errStr)
	}

	errStr = repo.Checkout(GitInitialCommit).Error()
	if errStr != "checkout failed:\nfatal: Not a git repository: '.git'" && errStr != "checkout failed:\nfatal: Not a git repository (or any of the parent directories): .git" {
		t.Fatalf("Unexpected error: %s", errStr)
	}
}

// Private stuff.

func setup(t *testing.T, tmpDir string) {
	_, code, err := internal.Capture(tmpDir, nil, "git", "init")
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)
	// This is needed explicitly on drone.io. I can only assume they use a global
	// template which inhibits the default branch name.
	_, code, err = internal.Capture(tmpDir, nil, "git", "checkout", "-b", "master")
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)
	run(t, tmpDir, nil, "config", "user.email", "nobody@localhost")
	run(t, tmpDir, nil, "config", "user.name", "nobody")
}

func check(t *testing.T, repo Repo, untracked []string, unstaged []string) {
	actualUntracked, err := repo.Untracked()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, untracked, actualUntracked)
	actualUnstaged, err := repo.Unstaged()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, unstaged, actualUnstaged)
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
