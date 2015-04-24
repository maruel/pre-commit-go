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

// Checks exists purely to manage YAML serialization.
type Checks struct {
	All []Check
}

func (c Checks) MarshalYAML() (interface{}, error) {
	data, err := yaml.Marshal(c.All)
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
	config := &Config{
		Version:     1,
		MaxDuration: 5,
		Checks:      map[RunLevel]Checks{},
	}
	config.Checks[RunLevel(0)] = Checks{[]Check{}}
	config.Checks[RunLevel(1)] = Checks{[]Check{
		&Build{},
		&Gofmt{},
		&Test{},
	}}
	config.Checks[RunLevel(2)] = Checks{[]Check{
		&Errcheck{},
		&Goimports{},
		&TestCoverage{},
	}}
	config.Checks[RunLevel(3)] = Checks{[]Check{
		&Golint{},
		&Govet{},
	}}
	for _, c := range config.AllChecks() {
		c.ResetDefault()
	}
	return config
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
		} else {
			if config.Version != 1 {
				log.Printf("Unsupported config version")
				config = New()
			}
		}
	}
	return config
}

// AllChecks returns all the checks.
func (c *Config) AllChecks() []Check {
	out := []Check{}
	for _, list := range c.Checks {
		for _, c := range list.All {
			out = append(out, c)
		}
	}
	return out
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
