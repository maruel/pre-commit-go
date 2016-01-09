// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Configuration.

package checks

import (
	"fmt"

	"github.com/maruel/pre-commit-go/Godeps/_workspace/src/gopkg.in/yaml.v2"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
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

	// MaxConcurrent, if not zero, is the maximum number of concurrent processes
	// to run. If zero, there is no maximum.
	MaxConcurrent int `yaml:"-"`
}

// EnabledChecks returns all the checks enabled.
func (c *Config) EnabledChecks(modes []Mode) ([]Check, *Options) {
	out := []Check{}
	options := &Options{}
	if c.MaxConcurrent > 0 {
		// Allocate and populate a run token semaphore.
		options.runTokens = make(chan struct{}, c.MaxConcurrent)
		for i := 0; i < c.MaxConcurrent; i++ {
			options.runTokens <- struct{}{}
		}
	}

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

	// runTokens is a channel containing "run tokens". Each task wishing to self-
	// meter should lease a run token prior to execution and return it afterwards.
	//
	// If nil, run token operations are no-ops.
	runTokens chan struct{}
}

// LeaseRunToken returns a leased run token.
//
// A token must be returned after use via ReturnRunToken. This should be done
// via defer, as failure to return a run token will result in throttling or
// deadlock.
func (o *Options) LeaseRunToken() {
	if o.runTokens == nil {
		return
	}
	<-o.runTokens
}

// ReturnRunToken returns a leased run token.
func (o *Options) ReturnRunToken() {
	if o.runTokens == nil {
		return
	}
	o.runTokens <- struct{}{}
}

// Capture sets GOPATH and executes a subprocess.
func (o *Options) Capture(r scm.ReadOnlyRepo, args ...string) (string, int, error) {
	o.LeaseRunToken()
	defer o.ReturnRunToken()

	return internal.Capture(r.Root(), []string{"GOPATH=" + r.GOPATH()}, args...)
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
