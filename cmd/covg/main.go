// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// covg: yet another coverage tool.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/maruel/pre-commit-go/checks"
	"github.com/maruel/pre-commit-go/checks/definitions"
	"github.com/maruel/pre-commit-go/scm"
)

var silentError = errors.New("silent error")

func printProfile(settings *definitions.CoverageSettings, profile checks.CoverageProfile, indent string) bool {
	maxLoc := 0
	maxName := 0
	for _, item := range profile {
		if item.Percent < 100. {
			if l := len(item.SourceRef()); l > maxLoc {
				maxLoc = l
			}
			if l := len(item.Name); l > maxName {
				maxName = l
			}
		}
	}
	for _, item := range profile {
		if item.Percent < 100. {
			fmt.Printf("%s%-*s %-*s %4.1f%% (%d/%d)\n", indent, maxLoc, item.SourceRef(), maxName, item.Name, item.Percent, item.Count, item.Total)
		}
	}
	if err := profile.Passes(settings); err != nil {
		fmt.Printf("%s%s\n", indent, err)
		return false
	}
	fmt.Printf("%s%3.1f%% (%d/%d) >= %.1f%%; Functions: %d untested / %d partially / %d completely\n",
		indent, profile.CoveragePercent(), profile.TotalCoveredLines(), profile.TotalLines(), settings.MinCoverage, profile.NonCoveredFuncs(), profile.PartiallyCoveredFuncs(), profile.CoveredFuncs())
	return true
}

func mainImpl() error {
	// TODO(maruel): Add support to use the same diff as pre-commit-go.
	minFlag := flag.Float64("min", 0, "minimum expected coverage in %")
	maxFlag := flag.Float64("max", 100, "maximum expected coverage in %")
	globalFlag := flag.Bool("g", false, "use global coverage")
	flag.Parse()

	log.SetOutput(ioutil.Discard)

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := scm.GetRepo(cwd, "")
	if err != nil {
		return err
	}
	// TODO(maruel): Run only tests down the current directory.
	if err := os.Chdir(repo.Root()); err != nil {
		return err
	}

	c := checks.Coverage{
		Global: definitions.CoverageSettings{
			MinCoverage: *minFlag,
			MaxCoverage: *maxFlag,
		},
	}
	// TODO(maruel): Run tests ala pre-commit-go; e.g. determine what diff to use.
	change, err := repo.Between(scm.Current, scm.GitInitialCommit, nil)
	if err != nil {
		return err
	}
	profile, err := c.RunProfile(change)
	if err != nil {
		return err
	}

	if *globalFlag {
		if !printProfile(&c.Global, profile, "") {
			return silentError
		}
	} else {
		for _, pkg := range change.All().Packages() {
			d := pkgToDir(pkg)
			subset := profile.Subset(d)
			if len(subset) != 0 {
				fmt.Printf("%s\n", d)
				if !printProfile(&c.Global, subset, "  ") {
					err = silentError
				}
			}
		}
	}
	return nil
}

func main() {
	if err := mainImpl(); err != nil {
		if err != silentError {
			fmt.Fprintf(os.Stderr, "covg: %s\n", err)
		}
		os.Exit(1)
	}
}

func pkgToDir(p string) string {
	if p == "." {
		return p
	}
	return p[2:]
}
