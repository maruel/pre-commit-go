// Copyright 2016 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package checks

import (
	"errors"
	"testing"

	"github.com/maruel/ut"
	"gopkg.in/yaml.v2"
)

func TestConfigNew(t *testing.T) {
	config := New("0.1")
	ut.AssertEqual(t, 3, len(config.Modes[PreCommit].Checks))
	ut.AssertEqual(t, 3, len(config.Modes[PrePush].Checks))
	ut.AssertEqual(t, 5, len(config.Modes[ContinuousIntegration].Checks))
	ut.AssertEqual(t, 3, len(config.Modes[Lint].Checks))
	checks, options := config.EnabledChecks([]Mode{PreCommit, PrePush, ContinuousIntegration, Lint})
	ut.AssertEqual(t, Options{MaxDuration: 120}, *options)
	ut.AssertEqual(t, 2+4+5+3, len(checks))
}

func TestConfigYAML(t *testing.T) {
	config := New("0.1")
	data, err := yaml.Marshal(config)
	ut.AssertEqual(t, nil, err)
	actual := &Config{}
	ut.AssertEqual(t, nil, yaml.Unmarshal(data, actual))
	ut.AssertEqual(t, config, actual)
}

func TestConfigYAMLBadMode(t *testing.T) {
	data, err := yaml.Marshal("foo")
	ut.AssertEqual(t, nil, err)
	v := PreCommit
	ut.AssertEqual(t, errors.New("invalid mode \"foo\""), yaml.Unmarshal(data, &v))
	ut.AssertEqual(t, PreCommit, v)
}
