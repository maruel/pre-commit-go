// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// covg: yet another coverage tool.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/maruel/pre-commit-go/checks"
	"github.com/maruel/pre-commit-go/scm"
)

func mainImpl() error {
	// TODO(maruel): Add support to use the same diff as pre-commit-go.
	minFlag := flag.Float64("min", 0, "minimum expected coverage in %")
	maxFlag := flag.Float64("max", 100, "maximum expected coverage in %")
	flag.Parse()

	log.SetOutput(ioutil.Discard)

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := scm.GetRepo(cwd)
	if err != nil {
		return err
	}
	// TODO(maruel): Run only tests down the current directory.
	if err := os.Chdir(repo.Root()); err != nil {
		return err
	}

	c := checks.Coverage{
		MinCoverage: *minFlag,
		MaxCoverage: *maxFlag,
	}
	// TODO(maruel): Run tests ala pre-commit-go.
	change, err := repo.Between(scm.Current, scm.GitInitialCommit, nil)
	if err != nil {
		return err
	}
	profile, err := c.RunProfile(change)
	if err != nil {
		return err
	}
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
			fmt.Printf("%-*s %-*s %1.1f%%\n", maxLoc, item.SourceRef(), maxName, item.Name, item.Percent)
		}
	}
	total := profile.Coverage()
	partial := profile.PartiallyCoveredFuncs()
	if total < c.MinCoverage {
		return fmt.Errorf("coverage: %3.1f%% < %.1f%%; %d untested functions", total, c.MinCoverage, partial)
	} else if c.MaxCoverage > 0 && total > c.MaxCoverage {
		return fmt.Errorf("coverage: %3.1f%% > %.1f%%; %d untested functions; please update \"max_coverage\"", total, c.MaxCoverage, partial)
	}
	fmt.Printf("coverage: %3.1f%% >= %.1f%%; %d untested functions\n", total, c.MinCoverage, partial)
	return nil
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "covg: %s\n", err)
		os.Exit(1)
	}
}
