// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Configuration.

package checks

import (
	"crypto/sha1"
	"fmt"

	"github.com/maruel/pre-commit-go/Godeps/_workspace/src/gopkg.in/yaml.v2"
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

// UnmarshalYAML implements yaml.Unmarshaler.
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
	return fmt.Errorf("invalid mode \"%s\"", val)
}

// Config is the serialized form of pre-commit-go.yml.
type Config struct {
	// MinVersion is set to the current pcg version. Earlier version will refuse
	// to load this file.
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
func (c *Config) EnabledChecks(modes []Mode) ([]Check, *Options) {
	out := []Check{}
	options := &Options{}
	for _, mode := range modes {
		for _, checks := range c.Modes[mode].Checks {
			out = append(out, checks...)
		}
		options = options.merge(c.Modes[mode].Options)
	}
	return out, options
}

// Settings is the settings used for a mode.
type Settings struct {
	// Checks is a map of all checks enabled for this mode, with the key being
	// the check type.
	Checks  Checks  `yaml:"checks"`
	Options Options `yaml:",inline"`
}

// Options hold the settings for a mode shared by all checks.
type Options struct {
	// MaxDuration is the maximum allowed duration to run all the checks in
	// seconds. If it takes more time than that, it is marked as failed.
	MaxDuration int `yaml:"max_duration"`

	// CurrentShard is the current shard value being executed.
	CurrentShard int `yaml:"-"`
	// TotalShards is the tutal number of shards being executed. If zero, this
	// execution is not being sharded.
	TotalShards int `yaml:"-"`
}

// merge merges two options and returns a result.
// This is used for multimode runs.
func (o *Options) merge(r Options) *Options {
	out := &Options{MaxDuration: o.MaxDuration}
	if out.MaxDuration < r.MaxDuration {
		out.MaxDuration = r.MaxDuration
	}
	return out
}

// isEnabled returns true if the named configuration option is enabled.
//
// This is determined by examining the sharding options. The named
// configuration option is enabled if it maps to the current shard.
func (o *Options) isEnabled(name string) bool {
	if o.TotalShards <= 1 {
		return true
	}

	// Condense the hash of "name" into an int64.
	value := int64(0)
	for i, b := range sha1.Sum([]byte(name)) {
		value ^= (int64(b) << (8 * (uint(i) % 8)))
	}
	return (int64(o.CurrentShard) == (value % int64(o.TotalShards)))
}

// isSingleton returns true if this is a singleton shard. Non-sharded Checks
// should test this to avoid running once for each shard.
func (o *Options) isSingleton() bool {
	return o.CurrentShard == 0
}

// enabledValues returns the list of supplied strings that map to the configured
// shard.
func (o *Options) enabledValues(s ...string) []string {
	r := make([]string, 0, len(s))
	for _, v := range s {
		if o.isEnabled(v) {
			r = append(r, v)
		}
	}
	return r
}

// Checks helps with Check serialization.
type Checks map[string][]Check

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *Checks) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var encoded map[string][]map[string]interface{}
	if err := unmarshal(&encoded); err != nil {
		return err
	}
	*c = Checks{}
	for checkTypeName, checks := range encoded {
		checkFactory, ok := KnownChecks[checkTypeName]
		if !ok {
			return fmt.Errorf("unknown check \"%s\"", checkTypeName)
		}
		for _, checkData := range checks {
			rawCheckData, err := yaml.Marshal(checkData)
			if err != nil {
				return err
			}
			check := checkFactory()
			if err = yaml.Unmarshal(rawCheckData, check); err != nil {
				return err
			}
			(*c)[checkTypeName] = append((*c)[checkTypeName], check)
		}
	}
	return nil
}

// New returns a default initialized Config instance.
func New(v string) *Config {
	return &Config{
		MinVersion: v,
		Modes: map[Mode]Settings{
			PreCommit: {
				Options: Options{MaxDuration: 5},
				Checks: Checks{
					"build": {
						&Build{
							BuildAll:  false,
							ExtraArgs: []string{},
						},
					},
					"gofmt": {
						&Gofmt{},
					},
					"test": {
						&Test{
							ExtraArgs: []string{"-short"},
						},
					},
				},
			},
			PrePush: {
				Options: Options{MaxDuration: 15},
				Checks: Checks{
					"goimports": {
						&Goimports{},
					},
					"coverage": {
						&Coverage{
							UseCoveralls: false,
							Global: CoverageSettings{
								MinCoverage: 50,
								MaxCoverage: 100,
							},
							PerDirDefault: CoverageSettings{
								MinCoverage: 1,
								MaxCoverage: 100,
							},
							PerDir: map[string]*CoverageSettings{},
						},
					},
					"test": {
						&Test{
							ExtraArgs: []string{"-v", "-race"},
						},
					},
				},
			},
			ContinuousIntegration: {
				Options: Options{MaxDuration: 120},
				Checks: Checks{
					"build": {
						&Build{
							BuildAll:  false,
							ExtraArgs: []string{},
						},
					},
					"gofmt": {
						&Gofmt{},
					},
					"goimports": {
						&Goimports{},
					},
					"coverage": {
						&Coverage{
							UseCoveralls: true,
							Global: CoverageSettings{
								MinCoverage: 50,
								MaxCoverage: 100,
							},
							PerDirDefault: CoverageSettings{
								MinCoverage: 1,
								MaxCoverage: 100,
							},
							PerDir: map[string]*CoverageSettings{},
						},
					},
					"test": {
						&Test{
							ExtraArgs: []string{"-v", "-race"},
						},
					},
				},
			},
			Lint: {
				Options: Options{MaxDuration: 15},
				Checks: Checks{
					"errcheck": {
						&Errcheck{
							// "Close|Write.*|Flush|Seek|Read.*"
							Ignores: "Close",
						},
					},
					"golint": {
						&Golint{
							Blacklist: []string{},
						},
					},
					"govet": {
						&Govet{
							Blacklist: []string{" composite literal uses unkeyed fields"},
						},
					},
				},
			},
		},
		IgnorePatterns: []string{
			".*",          // SCM
			"_*",          // Godeps
			"*.pb.go",     // protobuf
			"*_string.go", // stringer
		},
	}
}
