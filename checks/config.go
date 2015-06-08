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

// Category is one of the check type.
type Category string

const (
	PreCommit             Category = "pre-commit"
	PrePush               Category = "pre-push"
	ContinuousIntegration Category = "continuous-integration"
	Lint                  Category = "lint"
)

// AllCategories are all valid categories.
var AllCategories = []Category{PreCommit, PrePush, ContinuousIntegration, Lint}

func (c *Category) UnmarshalYAML(unmarshal func(interface{}) error) error {
	s := ""
	if err := unmarshal(&s); err != nil {
		return err
	}
	val := Category(s)
	for _, known := range AllCategories {
		if val == known {
			*c = val
			return nil
		}
	}
	return fmt.Errorf("invalid category \"%s\"", *c)
}

type Config struct {
	Version int                           `yaml:"version"` // Should be incremented when it's not compatible anymore.
	Modes   map[Category]CategorySettings `yaml:"modes"`   // Checks per category.
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	data := &struct {
		Version int                           `yaml:"version"`
		Modes   map[Category]CategorySettings `yaml:"modes"`
	}{}
	if err := unmarshal(data); err != nil {
		return err
	}
	if data.Version != currentVersion {
		return fmt.Errorf("unexpected version %d, expected %d", data.Version, currentVersion)
	}
	c.Version = data.Version
	c.Modes = data.Modes
	return nil
}

// CategorySettings is the settings used for a category.
type CategorySettings struct {
	Checks      Checks `yaml:"checks"`
	MaxDuration int    `yaml:"max_duration"` // In seconds.
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
func New() *Config {
	return &Config{
		Version: currentVersion,
		Modes: map[Category]CategorySettings{
			PreCommit: {
				MaxDuration: 5,
				Checks: Checks{
					&gofmt{},
					&test{
						ExtraArgs: []string{"-short"},
					},
				},
			},
			PrePush: {
				MaxDuration: 15,
				Checks: Checks{
					&build{
						ExtraArgs: []string{},
					},
					&goimports{},
					&testCoverage{
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
					&testCoverage{
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

// EnabledChecks returns all the checks enabled.
func (c *Config) EnabledChecks(categories []Category) ([]Check, int) {
	max := 0
	out := []Check{}
	for _, category := range categories {
		out = append(out, c.Modes[category].Checks...)
		if c.Modes[category].MaxDuration > max {
			max = c.Modes[category].MaxDuration
		}
	}
	return out, max
}

// Private stuff.

const currentVersion = 2
