// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"errors"
	"testing"

	"github.com/maruel/ut"
	"gopkg.in/yaml.v2"
)

func TestRunLevelUnmarshalYAML1(t *testing.T) {
	data := []byte("foo: 1")
	out := &struct {
		Foo RunLevel
	}{}
	ut.AssertEqual(t, nil, yaml.Unmarshal(data, out))
	ut.AssertEqual(t, RunLevel(1), out.Foo)
}

func TestRunLevelUnmarshalYAML4(t *testing.T) {
	data := []byte("foo: 4")
	out := &struct {
		Foo RunLevel
	}{}
	ut.AssertEqual(t, errors.New("invalid runlevel 4"), yaml.Unmarshal(data, out))
	ut.AssertEqual(t, RunLevel(0), out.Foo)
}
