// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build go1.4

package scm

import (
	"os"
	"strings"
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
