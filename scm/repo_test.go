// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/ut"
)

func TestGetRepoGitSlow(t *testing.T) {
	// Make a repository and test behavior against it.
	t.Parallel()
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := internal.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	_, _, err = internal.Capture(tmpDir, nil, "git", "init")
	ut.AssertEqual(t, nil, err)
	run(t, tmpDir, nil, "config", "user.email", "nobody@localhost")
	run(t, tmpDir, nil, "config", "user.name", "nobody")
	repo, err := GetRepo(tmpDir)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, tmpDir, repo.Root())
	p, err := repo.PreCommitHookPath()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, filepath.Join(tmpDir, ".git", "hooks", "pre-commit"), p)
	ut.AssertEqual(t, "4b825dc642cb6eb9a060e54bf8d69288fbee4904", repo.HEAD())

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
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))
	head := repo.HEAD()
	// TODO(maruel): Figure out what makes it slightly non-deterministic.
	if head != "8d894d29f11f8947a7d82221d579f09f7cb6c9eb" && head != "56e6926b12ee571cfba4515214725b35a8571570" {
		t.Fatalf("%s", run(t, tmpDir, nil, "log", "-p", "--format=fuller"))
	}

	done, err = repo.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, true, done)
	ut.AssertEqual(t, "hi\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, nil, repo.Restore())
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))
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

	_, _, err = internal.Capture(tmpDir, nil, "git", "init")
	ut.AssertEqual(t, nil, err)
	run(t, tmpDir, nil, "config", "user.email", "nobody@localhost")
	run(t, tmpDir, nil, "config", "user.name", "nobody")
	repo, err := GetRepo(tmpDir)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, tmpDir, repo.Root())
	ut.AssertEqual(t, nil, internal.RemoveAll(filepath.Join(tmpDir, ".git")))

	p, err := repo.PreCommitHookPath()
	ut.AssertEqual(t, errors.New("failed to find .git dir: failed to find .git dir: failed to run \"git rev-parse --git-dir\""), err)
	ut.AssertEqual(t, "", p)

	untracked, err := repo.Untracked()
	ut.AssertEqual(t, errors.New("failed to retrieve untracked files"), err)
	ut.AssertEqual(t, []string(nil), untracked)

	unstaged, err := repo.Unstaged()
	ut.AssertEqual(t, errors.New("failed to retrieve unstaged files"), err)
	ut.AssertEqual(t, []string(nil), unstaged)
}

// Private stuff.

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
