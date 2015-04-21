// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Configuration.

package checks

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

// RunLevel is between 0 and 3.
//
// [0, 3]. 0 is never, 3 is always. Default:
//   - most checks that only require the stdlib have default RunLevel of 1
//   - most checks that require third parties have default RunLevel of 2
//   - checks that may trigger false positives have default RunLevel of 3
type RunLevel int

type Config struct {
	MaxDuration int // In seconds.

	// Checks per run level.
	Checks map[RunLevel][]Check
}

// GetConfig returns a Config with defaults set then loads the config from
// file "name".
func GetConfig(name string) *Config {
	config := &Config{MaxDuration: 120, Checks: map[RunLevel][]Check{}}
	config.Checks[RunLevel(0)] = []Check{}
	config.Checks[RunLevel(1)] = []Check{
		&Build{},
		&Gofmt{},
		&Test{},
	}
	config.Checks[RunLevel(2)] = []Check{
		&Errcheck{},
		&Goimports{},
		&TestCoverage{},
	}
	config.Checks[RunLevel(3)] = []Check{
		&Golint{},
		&Govet{},
	}
	for _, c := range config.AllChecks() {
		c.ResetDefault()
	}

	content, err := ioutil.ReadFile(name)
	if err == nil {
		if err2 := yaml.Unmarshal(content, config); err2 != nil {
			log.Printf("failed to parse %s: %s", name, err2)
		}
	}
	return config
}

// AllChecks returns all the checks.
func (c *Config) AllChecks() []Check {
	out := []Check{}
	for _, list := range c.Checks {
		for _, c := range list {
			out = append(out, c)
		}
	}
	return out
}

// EnabledChecks returns all the checks enabled at this run level.
func (c *Config) EnabledChecks(r RunLevel) []Check {
	out := []Check{}
	for i := RunLevel(0); i <= r; i++ {
		for _, c := range c.Checks[i] {
			out = append(out, c)
		}
	}
	return out
}
