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

func TestConfigNew(t *testing.T) {
	config := New()
	ut.AssertEqual(t, 2, len(config.Modes[PreCommit].Checks))
	ut.AssertEqual(t, 4, len(config.Modes[PrePush].Checks))
	ut.AssertEqual(t, 5, len(config.Modes[ContinuousIntegration].Checks))
	ut.AssertEqual(t, 3, len(config.Modes[Lint].Checks))
	checks, max := config.EnabledChecks([]Category{PreCommit, PrePush, ContinuousIntegration, Lint})
	ut.AssertEqual(t, 120, max)
	ut.AssertEqual(t, 2+4+5+3, len(checks))
}

func TestConfigYAML(t *testing.T) {
	config := New()
	data, err := yaml.Marshal(config)
	ut.AssertEqual(t, nil, err)
	actual := &Config{}
	ut.AssertEqual(t, nil, yaml.Unmarshal(data, actual))
	ut.AssertEqual(t, config, actual)
}

func TestConfigVersion(t *testing.T) {
	data, err := yaml.Marshal(&Config{Version: currentVersion - 1})
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, errors.New("unexpected version 1, expected 2"), yaml.Unmarshal(data, &Config{}))
}
