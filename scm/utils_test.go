// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package scm

import (
	"errors"
	"os"
	"testing"

	"github.com/maruel/pre-commit-go/Godeps/_workspace/src/github.com/maruel/ut"
)

func TestRelToGOPATH(t *testing.T) {
	t.Parallel()
	p, err := relToGOPATH("foo", string(os.PathListSeparator))
	ut.AssertEqual(t, "", p)
	ut.AssertEqual(t, errors.New("failed to find GOPATH relative directory for foo"), err)
}
