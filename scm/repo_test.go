// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/ut"
)

func TestGetRepoGitSlow(t *testing.T) {
	// Make a repository and test behavior against it.
	tmpDir, err := ioutil.TempDir("", "pre-commit-go")
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("%s", err)
		}
	}()

	_, _, err = internal.CaptureWd(tmpDir, "git", "init")
	ut.AssertEqual(t, nil, err)
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

	run(t, tmpDir, "add", "file1")
	check(t, repo, nil, nil)

	done, err := repo.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, false, done)

	write(t, tmpDir, "file1", "hi\nhello\n")
	check(t, repo, nil, []string{"file1"})

	done, err = repo.Stash()
	ut.AssertEqual(t, errors.New("Can't stash until there's at least one commit"), err)
	ut.AssertEqual(t, false, done)

	run(t, tmpDir, "commit", "-m", "yo")
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))

	done, err = repo.Stash()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, true, done)
	ut.AssertEqual(t, "hi\n", read(t, tmpDir, "file1"))
	ut.AssertEqual(t, nil, repo.Restore())
	ut.AssertEqual(t, "hi\nhello\n", read(t, tmpDir, "file1"))
}

func check(t *testing.T, repo Repo, untracked []string, unstaged []string) {
	actualUntracked, err := repo.Untracked()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, untracked, actualUntracked)
	actualUnstaged, err := repo.Unstaged()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, unstaged, actualUnstaged)
}

func run(t *testing.T, tmpDir string, args ...string) {
	internal := &git{root: tmpDir}
	_, code, err := internal.capture(args...)
	ut.AssertEqual(t, 0, code)
	ut.AssertEqual(t, nil, err)
}

func write(t *testing.T, tmpDir, name string, content string) {
	ut.AssertEqual(t, nil, ioutil.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0600))
}

func read(t *testing.T, tmpDir, name string) string {
	content, err := ioutil.ReadFile(filepath.Join(tmpDir, name))
	ut.AssertEqual(t, nil, err)
	return string(content)
}
