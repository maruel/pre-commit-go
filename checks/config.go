// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Configuration.

package checks

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"

	"gopkg.in/yaml.v2"
)

// RunLevel is between 0 and 3.
//
// [0, 3]. 0 is never, 3 is always. Default:
//   - most checks that only require the stdlib have default RunLevel of 1
//   - most checks that require third parties have default RunLevel of 2
//   - checks that may trigger false positives have default RunLevel of 3
type RunLevel int

func (r *RunLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	level := 0
	if err := unmarshal(&level); err != nil {
		return err
	}
	if level < 0 || level > 3 {
		return fmt.Errorf("invalid runlevel %d", level)
	}
	*r = RunLevel(level)
	return nil
}

type Config struct {
	Version     int                 `yaml:"version"`      // Should be incremented when it's not compatible anymore.
	MaxDuration int                 `yaml:"max_duration"` // In seconds.
	Checks      map[RunLevel]Checks `yaml:"checks"`       // Checks per run level.
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	data := &struct {
		Version     int                 `yaml:"version"`
		MaxDuration int                 `yaml:"max_duration"`
		Checks      map[RunLevel]Checks `yaml:"checks"`
	}{}
	if err := unmarshal(data); err != nil {
		return err
	}
	if data.Version != currentVersion {
		return fmt.Errorf("unexpected version %d, expected %d", data.Version, currentVersion)
	}
	c.Version = data.Version
	c.MaxDuration = data.MaxDuration
	c.Checks = data.Checks
	return nil
}

// Checks exists purely to manage YAML serialization.
type Checks struct {
	All []Check
}

func (c Checks) MarshalYAML() (interface{}, error) {
	all := c.All
	if all == nil {
		all = []Check{}
	}
	data, err := yaml.Marshal(all)
	if err != nil {
		return nil, err
	}
	var encoded []map[string]interface{}
	if err = yaml.Unmarshal(data, &encoded); err != nil {
		return nil, err
	}
	for i, item := range encoded {
		item["check_type"] = c.All[i].GetName()
	}
	return encoded, nil
}

func (c *Checks) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var encoded []map[string]interface{}
	if err := unmarshal(&encoded); err != nil {
		return err
	}
	c.All = []Check{}
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
		c.All = append(c.All, check)
	}
	return nil
}

func New() *Config {
	return &Config{
		Version:     currentVersion,
		MaxDuration: 120,
		Checks: map[RunLevel]Checks{
			RunLevel(0): {[]Check{}},
			RunLevel(1): {[]Check{
				&Build{
					ExtraArgs: []string{},
				},
				&Gofmt{},
				&Test{
					ExtraArgs: []string{"-v", "-race"},
				},
			}},
			RunLevel(2): {[]Check{
				&Errcheck{
					// "Close|Write.*|Flush|Seek|Read.*"
					Ignores: "Close",
				},
				&Goimports{},
				&TestCoverage{
					MinimumCoverage: 20.,
				},
			}},
			RunLevel(3): {[]Check{
				&Golint{
					Blacklist: []string{},
				},
				&Govet{
					Blacklist: []string{" composite literal uses unkeyed fields"},
				},
			}},
		},
	}
}

// GetConfig returns a Config with defaults set then loads the config from file
// "pathname".
func GetConfig(pathname string) *Config {
	config := New()

	content, err := ioutil.ReadFile(pathname)
	if err == nil {
		if err2 := yaml.Unmarshal(content, config); err2 != nil {
			// Log but ignore the error, recreate a new config instance.
			log.Printf("failed to parse %s: %s", pathname, err2)
			config = New()
		}
	}
	return config
}

// EnabledChecks returns all the checks enabled at this run level.
func (c *Config) EnabledChecks(r RunLevel) []Check {
	out := []Check{}
	for i := RunLevel(0); i <= r; i++ {
		for _, c := range c.Checks[i].All {
			out = append(out, c)
		}
	}
	return out
}

// Private stuff.

const currentVersion = 1
