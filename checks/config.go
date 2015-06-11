// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Configuration.

package checks

import (
	"errors"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v2"
)

// Mode is one of the check mode. When running checks, the mode determine what
// checks are executed.
type Mode string

// All predefined modes are executed automatically based on the context, except
// for Lint which needs to be selected manually.
const (
	PreCommit             Mode = "pre-commit"
	PrePush               Mode = "pre-push"
	ContinuousIntegration Mode = "continuous-integration"
	Lint                  Mode = "lint"
)

// AllModes are all known valid modes that can be used in pre-commit-go.yml.
var AllModes = []Mode{PreCommit, PrePush, ContinuousIntegration, Lint}

func (m *Mode) UnmarshalYAML(unmarshal func(interface{}) error) error {
	s := ""
	if err := unmarshal(&s); err != nil {
		return err
	}
	val := Mode(s)
	for _, known := range AllModes {
		if val == known {
			*m = val
			return nil
		}
	}
	return fmt.Errorf("invalid mode \"%s\"", *m)
}

// Config is the serialized form of pre-commit-go.yml.
type Config struct {
	// MinVersion is set to the current pre-commit-go version. Earlier version
	// will refuse to load this file.
	MinVersion string `yaml:"min_version"`
	// Settings per mode. Settings includes the checks and the maximum allowed
	// time spent to run them.
	Modes map[Mode]Settings `yaml:"modes"`
	// IgnorePatterns is all paths glob patterns that should be ignored. By
	// default, this include any file or directory starting with "." or "_", i.e.
	// []string{".*", "_*"}.  This is a glob that is applied to each path
	// component of each file.
	IgnorePatterns []string `yaml:"ignore_patterns"`
}

// EnabledChecks returns all the checks enabled.
func (c *Config) EnabledChecks(modes []Mode) ([]Check, int) {
	max := 0
	out := []Check{}
	for _, mode := range modes {
		out = append(out, c.Modes[mode].Checks...)
		if c.Modes[mode].MaxDuration > max {
			max = c.Modes[mode].MaxDuration
		}
	}
	return out, max
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	data := &struct {
		MinVersion     string            `yaml:"min_version"`
		Modes          map[Mode]Settings `yaml:"modes"`
		IgnorePatterns []string          `yaml:"ignore_patterns"`
	}{}
	if err := unmarshal(data); err != nil {
		return err
	}
	c.MinVersion = data.MinVersion
	c.Modes = data.Modes
	c.IgnorePatterns = data.IgnorePatterns
	return nil
}

// Settings is the settings used for a mode.
type Settings struct {
	// Checks is the list of all checks enabled for this mode.
	Checks Checks `yaml:"checks"`
	// MaxDuration is the maximum allowed duration to run all the checks in
	// seconds. If it takes more time than that, it is marked as failed.
	MaxDuration int `yaml:"max_duration"`
}

// Checks helps with Check serialization.
type Checks []Check

func (c Checks) MarshalYAML() (interface{}, error) {
	data, err := yaml.Marshal([]Check(c))
	if err != nil {
		return nil, err
	}
	var encoded []map[string]interface{}
	if err = yaml.Unmarshal(data, &encoded); err != nil {
		return nil, err
	}
	for i, item := range encoded {
		item["check_type"] = c[i].GetName()
	}
	return encoded, nil
}

func (c *Checks) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var encoded []map[string]interface{}
	if err := unmarshal(&encoded); err != nil {
		return err
	}
	for _, checkData := range encoded {
		checkTypeNameRaw, ok := checkData["check_type"]
		if !ok {
			return errors.New("require key check_type")
		}
		delete(checkData, "check_type")
		checkTypeName, ok := checkTypeNameRaw.(string)
		if !ok {
			return errors.New("require key check_type to be a string")
		}
		checkType, ok := KnownChecks[checkTypeName]
		if !ok {
			return fmt.Errorf("unknown check \"%s\"", checkTypeName)
		}
		rawCheckData, err := yaml.Marshal(checkData)
		if err != nil {
			return err
		}
		check := reflect.New(reflect.TypeOf(checkType).Elem()).Interface().(Check)
		if err = yaml.Unmarshal(rawCheckData, check); err != nil {
			return err
		}
		*c = append(*c, check)
	}
	return nil
}

// New returns a default initialized Config instance.
func New(v string) *Config {
	return &Config{
		MinVersion: v,
		Modes: map[Mode]Settings{
			PreCommit: {
				MaxDuration: 5,
				Checks: Checks{
					&build{
						ExtraArgs: []string{},
					},
					&gofmt{},
					&test{
						ExtraArgs: []string{"-short"},
					},
				},
			},
			PrePush: {
				MaxDuration: 15,
				Checks: Checks{
					&goimports{},
					&coverage{
						MinimumCoverage: 60,
					},
					&test{
						ExtraArgs: []string{"-v", "-race"},
					},
				},
			},
			ContinuousIntegration: {
				MaxDuration: 120,
				Checks: Checks{
					&build{
						ExtraArgs: []string{},
					},
					&gofmt{},
					&goimports{},
					&coverage{
						MinimumCoverage: 60,
					},
					&test{
						ExtraArgs: []string{"-v", "-race"},
					},
				},
			},
			Lint: {
				MaxDuration: 15,
				Checks: Checks{
					&errcheck{
						// "Close|Write.*|Flush|Seek|Read.*"
						Ignores: "Close",
					},
					&golint{
						Blacklist: []string{},
					},
					&govet{
						Blacklist: []string{" composite literal uses unkeyed fields"},
					},
				},
			},
		},
		IgnorePatterns: []string{".*", "_*"},
	}
}
