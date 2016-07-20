// Copyright 2016 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/maruel/ut"
)

func TestRemoveAllMissing(t *testing.T) {
	td, err := ioutil.TempDir("", "pre-commit-go")
	ut.ExpectEqual(t, nil, err)

	foo := filepath.Join(td, "foo")
	err = ioutil.WriteFile(foo, []byte("yo"), 0600)
	ut.ExpectEqual(t, nil, err)
	ut.AssertEqual(t, nil, RemoveAll(td))
}

func TestRemoveAll(t *testing.T) {
	td, err := ioutil.TempDir("", "pre-commit-go")
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, RemoveAll(td))
}
