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
	t.Parallel()
	data := []byte("foo: 1")
	out := &struct {
		Foo RunLevel
	}{}
	ut.AssertEqual(t, nil, yaml.Unmarshal(data, out))
	ut.AssertEqual(t, RunLevel(1), out.Foo)
}

func TestRunLevelUnmarshalYAML4(t *testing.T) {
	t.Parallel()
	data := []byte("foo: 4")
	out := &struct {
		Foo RunLevel
	}{}
	ut.AssertEqual(t, errors.New("invalid runlevel 4"), yaml.Unmarshal(data, out))
	ut.AssertEqual(t, RunLevel(0), out.Foo)
}

func TestRunLevelUnmarshalYAMLBad(t *testing.T) {
	t.Parallel()
	data := []byte("foo: hi")
	out := &struct {
		Foo RunLevel
	}{}
	ut.AssertEqual(t, &yaml.TypeError{Errors: []string{"line 1: cannot unmarshal !!str `hi` into int"}}, yaml.Unmarshal(data, out))
	ut.AssertEqual(t, RunLevel(0), out.Foo)
}

func TestConfigNew(t *testing.T) {
	config := New()
	ut.AssertEqual(t, 0, len(config.Checks[RunLevel(0)].All))
	ut.AssertEqual(t, 3, len(config.Checks[RunLevel(1)].All))
	ut.AssertEqual(t, 3, len(config.Checks[RunLevel(2)].All))
	ut.AssertEqual(t, 2, len(config.Checks[RunLevel(3)].All))
	for _, check := range config.AllChecks() {
		ut.AssertEqual(t, true, check.GetDescription() != "")
	}
}

func TestConfigYAML(t *testing.T) {
	config := New()
	data, err := yaml.Marshal(config)
	ut.AssertEqual(t, nil, err)
	actual := &Config{}
	ut.AssertEqual(t, nil, yaml.Unmarshal(data, actual))
	ut.AssertEqual(t, config, actual)
}
