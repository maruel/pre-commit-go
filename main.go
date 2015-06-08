// Copyright 2015 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// pre-commit-go: runs pre-commit checks on Go projects.
//
// See https://github.com/maruel/pre-commit-go for more details.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/maruel/pre-commit-go/checks"
	"github.com/maruel/pre-commit-go/checks/definitions"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
	"gopkg.in/yaml.v2"
)

// Globals

// Bump when the CLI or behavior change in any significant way.
const version = "0.2"

const hookContent = `#!/bin/sh
# AUTOGENERATED BY pre-commit-go.
#
# For more information, run:
#   pre-commit-go help
#
# or visit https://github.com/maruel/pre-commit-go

set -e
pre-commit-go run-hook %s
`

const nilCommit = "0000000000000000000000000000000000000000"

// http://git-scm.com/docs/githooks#_pre_push
var rePrePush = regexp.MustCompile("^(.+?) ([0-9a-f]{40}) (.+?) ([0-9a-f]{40})$")

var helpText = template.Must(template.New("help").Parse(`pre-commit-go: runs pre-commit checks on Go projects, fast.

Supported commands are:
  help        - this page
  prereq      - installs prerequisites, e.g.: errcheck, golint, goimports,
                govet, etc as applicable for the enabled checks
  install     - runs 'prereq' then installs the git commit hook as
                .git/hooks/pre-commit
  installrun  - runs 'prereq', 'install' then 'run'
  run         - runs all enabled checks
  run-hook    - used by hooks (pre-commit, pre-push) exclusively
  version     - print the tool version number
  writeconfig - writes (or rewrite) a pre-commit-go.yml

When executed without command, it does the equivalent of 'installrun'.

Supported flags are:
{{.Usage}}
Supported checks:
  Native checks that only depends on the stdlib:{{range .NativeChecks}}
    - {{printf "%-*s" $.Max .GetName}} : {{.GetDescription}}{{end}}

  Checks that have prerequisites (which will be automatically installed):{{range .OtherChecks}}
    - {{printf "%-*s" $.Max .GetName}} : {{.GetDescription}}{{end}}

No check ever modify any file.
`))

const yamlHeader = `# https://github.com/maruel/pre-commit-go configuration file to run checks
# automatically on commit, on push and on continuous integration service after
# a push or on merge of a pull request.
#
# See https://godoc.org/github.com/maruel/pre-commit-go/checks for more
# information.

`

// Utils.

func callRun(check checks.Check) (error, time.Duration) {
	if l, ok := check.(sync.Locker); ok {
		l.Lock()
		defer l.Unlock()
	}
	start := time.Now()
	err := check.Run()
	return err, time.Now().Sub(start)
}

func runChecks(config *checks.Config, categories []checks.Category) error {
	// TODO(maruel): Run checks selectively based on the actual files modified.
	// This should affect checks.goDirs() results by calculating all packages
	// affected via the package import graphs.
	start := time.Now()
	enabledChecks, maxDuration := config.EnabledChecks(categories)
	var wg sync.WaitGroup
	errs := make(chan error, len(enabledChecks))
	for _, c := range enabledChecks {
		wg.Add(1)
		go func(check checks.Check) {
			defer wg.Done()
			log.Printf("%s...", check.GetName())
			err, duration := callRun(check)
			log.Printf("... %s in %1.2fs", check.GetName(), duration.Seconds())
			if err != nil {
				errs <- err
			}
			// A check that took too long is a check that failed.
			if duration > time.Duration(maxDuration)*time.Second {
				errs <- fmt.Errorf("check %s took %1.2fs", check.GetName(), duration.Seconds())
			}
		}(c)
	}
	wg.Wait()

	var err error
	for {
		select {
		case err = <-errs:
			fmt.Printf("%s\n", err)
		default:
			if err != nil {
				duration := time.Now().Sub(start)
				return fmt.Errorf("checks failed in %1.2fs", duration.Seconds())
			}
			return err
		}
	}
}

func runPreCommit(repo scm.Repo, config *checks.Config) error {
	// First, stash index and work dir, keeping only the to-be-committed changes
	// in the working directory.
	stashed, err := repo.Stash()
	if err != nil {
		return err
	}
	// Run the checks.
	err = runChecks(config, []checks.Category{checks.PreCommit})

	// If stashed is false, everything was in the index so no stashing was needed.
	if stashed {
		if err2 := repo.Restore(); err == nil {
			err = err2
		}
	}
	return err
}

func runPrePush(repo scm.Repo, config *checks.Config) (err error) {
	previous := repo.HEAD()
	previousRef := repo.Ref()
	curr := previous
	stashed := false
	defer func() {
		if curr != previous {
			p := previousRef
			if p == "" {
				p = previous
			}
			if err2 := repo.Checkout(p); err == nil {
				err = err2
			}
		}
		if stashed {
			if err2 := repo.Restore(); err == nil {
				err = err2
			}
		}
	}()

	bio := bufio.NewReader(os.Stdin)
	line := ""
	for {
		if line, err = bio.ReadString('\n'); err != nil {
			break
		}
		matches := rePrePush.FindStringSubmatch(line)
		if len(matches) != 5 {
			return errors.New("unexpected stdin for pre-push")
		}
		from := matches[5]
		to := matches[3]
		// If it's not being deleted.
		if to == nilCommit {
			continue
		}
		if to != curr {
			// Stash, checkout, run tests.
			if !stashed {
				if stashed, err = repo.Stash(); err != nil {
					return
				}
			}
			curr = to
			if err = repo.Checkout(to); err != nil {
				return
			}
		}
		if from == nilCommit {
			from = scm.GitInitialCommit
		}
		// TODO(maruel): Relative [from,to].
		err = runChecks(config, []checks.Category{checks.PrePush})
	}
	if err == io.EOF {
		err = nil
	}
	return
}

// processCategory returns "" if the category is not valid.
//
// It implements the shortnames.
func processCategory(c string) checks.Category {
	switch c {
	case string(checks.PreCommit), "fast", "pc":
		return checks.PreCommit
	case string(checks.PrePush), "slow", "pp":
		return checks.PrePush
	case string(checks.ContinuousIntegration), "full", "ci":
		return checks.ContinuousIntegration
	case string(checks.Lint):
		return checks.Lint
	default:
		return checks.Category("")
	}
}

func processCategories(categoryFlag string) ([]checks.Category, error) {
	if len(categoryFlag) == 0 {
		return nil, nil
	}
	var categories []checks.Category
	for _, p := range strings.Split(categoryFlag, ",") {
		if len(p) != 0 {
			c := processCategory(p)
			if c == "" {
				return nil, fmt.Errorf("invalid category \"%s\"\n\nSupported categories (with shortcut names):\n- pre-commit / fast / pc\n- pre-push / slow / pp  (default)\n- continous-integration / full / ci\n- lint", p)
			}
			categories = append(categories, c)
		}
	}
	return categories, nil
}

// Commands.

func cmdHelp(repo scm.Repo, config *checks.Config, usage string) error {
	s := &struct {
		Usage        string
		Max          int
		NativeChecks []checks.Check
		OtherChecks  []checks.Check
	}{
		usage,
		0,
		[]checks.Check{},
		[]checks.Check{},
	}
	for name, c := range checks.KnownChecks {
		if v := len(name); v > s.Max {
			s.Max = v
		}
		if len(c.GetPrerequisites()) == 0 {
			s.NativeChecks = append(s.NativeChecks, c)
		} else {
			s.OtherChecks = append(s.OtherChecks, c)
		}
	}
	return helpText.Execute(os.Stdout, s)
}

// cmdInstallPrereq installs all the packages needed to run the enabled checks.
func cmdInstallPrereq(repo scm.Repo, config *checks.Config, categories []checks.Category) error {
	var wg sync.WaitGroup
	enabledChecks, _ := config.EnabledChecks(categories)
	c := make(chan string, len(enabledChecks))
	for _, check := range enabledChecks {
		for _, p := range check.GetPrerequisites() {
			wg.Add(1)
			go func(prereq definitions.CheckPrerequisite) {
				defer wg.Done()
				if !prereq.IsPresent() {
					c <- prereq.URL
				}
			}(p)
		}
	}
	wg.Wait()
	loop := true
	// Use a map to remove duplicates.
	m := map[string]bool{}
	for loop {
		select {
		case url := <-c:
			m[url] = true
		default:
			loop = false
		}
	}
	urls := make([]string, 0, len(m))
	for url := range m {
		urls = append(urls, url)
	}
	sort.Strings(urls)
	if len(urls) != 0 {
		fmt.Printf("Installing:\n")
		for _, url := range urls {
			fmt.Printf("  %s\n", url)
		}

		// First try without -u, then with -u. The main reason is golint, which
		// changed its API around go1.3~1.4 time frame. -u slows things down
		// significantly so it's worth trying out without, and people will
		// generally do not like to have things upgraded behind them.
		out, _, err := internal.Capture("", nil, append([]string{"go", "get"}, urls...)...)
		if len(out) != 0 || err != nil {
			out, _, err = internal.Capture("", nil, append([]string{"go", "get", "-u"}, urls...)...)
		}
		if len(out) != 0 {
			return fmt.Errorf("prerequisites installation failed: %s", out)
		}
		if err != nil {
			return fmt.Errorf("prerequisites installation failed: %s", err)
		}
	}
	return nil
}

// cmdInstall first calls cmdInstallPrereq() then install the .git/hooks/pre-commit hook.
func cmdInstall(repo scm.Repo, config *checks.Config, categories []checks.Category) error {
	if err := cmdInstallPrereq(repo, config, categories); err != nil {
		return err
	}
	hookDir, err := repo.HookPath()
	if err != nil {
		return err
	}
	// TODO(maruel): Add "pre-push" once it's tested to work.
	for _, t := range []string{"pre-commit"} {
		// Always remove hook first if it exists, in case it's a symlink.
		p := filepath.Join(hookDir, t)
		_ = os.Remove(p)
		if err = ioutil.WriteFile(p, []byte(fmt.Sprintf(hookContent, t)), 0777); err != nil {
			return err
		}
	}
	log.Printf("installation done")
	return nil
}

// cmdRun runs all the enabled checks.
func cmdRun(repo scm.Repo, config *checks.Config, categories []checks.Category) error {
	return runChecks(config, categories)
}

// cmdRunHook runs the checks in a git repository.
//
// Use a precise "stash, run checks, unstash" to ensure that the check is
// properly run on the data in the index.
func cmdRunHook(repo scm.Repo, config *checks.Config, mode string) error {
	switch checks.Category(mode) {
	case checks.PreCommit:
		return runPreCommit(repo, config)

	case checks.PrePush:
		return runPrePush(repo, config)

	case checks.ContinuousIntegration:
		return runChecks(config, []checks.Category{checks.ContinuousIntegration})

	default:
		return errors.New("unsupported hook type for run-hook")
	}
}

func cmdWriteConfig(repo scm.Repo, config *checks.Config, configPath string) error {
	content, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("internal error when marshaling config: %s", err)
	}
	_ = os.Remove(configPath)
	return ioutil.WriteFile(configPath, append([]byte(yamlHeader), content...), 0666)
}

func mainImpl() error {
	cmd := ""
	if len(os.Args) == 1 {
		cmd = "installrun"
	} else {
		cmd = os.Args[1]
		copy(os.Args[1:], os.Args[2:])
		os.Args = os.Args[:len(os.Args)-1]
	}
	verbose := flag.Bool("verbose", false, "enables verbose logging output")
	configPath := flag.String("config", "pre-commit-go.yml", "file name of the config to load")
	categoryFlag := flag.String("mode", "", "coma separated list of categories to process; default depends on the command")
	flag.StringVar(categoryFlag, "M", "", "shortcut to -mode")
	// Ignored, keep for a moment for compatibility with older clients.
	// TODO(maruel): Remove.
	_ = flag.Int("level", 0, "deprecated, unused")
	flag.Parse()

	log.SetFlags(log.Lmicroseconds)
	if !*verbose {
		log.SetOutput(ioutil.Discard)
	}

	categories, err := processCategories(*categoryFlag)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := scm.GetRepo(cwd)
	if err != nil {
		return err
	}
	if err := os.Chdir(repo.Root()); err != nil {
		return err
	}

	// Load the config.
	var config *checks.Config
	if filepath.IsAbs(*configPath) {
		config = checks.LoadConfig(*configPath)
	} else {
		if config = checks.LoadConfig(filepath.Join(".git", *configPath)); config == nil {
			config = checks.LoadConfig(*configPath)
		}
	}
	if config == nil {
		// Default config.
		config := checks.New()
	}

	switch cmd {
	case "help", "-help", "-h":
		b := &bytes.Buffer{}
		flag.CommandLine.SetOutput(b)
		flag.CommandLine.PrintDefaults()
		return cmdHelp(repo, config, b.String())
	case "install", "i":
		if len(categories) == 0 {
			categories = checks.AllCategories
		}
		return cmdInstall(repo, config, categories)
	case "installrun":
		if len(categories) == 0 {
			if checks.IsContinuousIntegration() {
				categories = []checks.Category{checks.ContinuousIntegration}
			} else {
				categories = []checks.Category{checks.PrePush}
			}
		}
		if err := cmdInstall(repo, config, categories); err != nil {
			return err
		}
		return cmdRun(repo, config, categories)
	case "prereq", "p":
		if len(categories) == 0 {
			categories = checks.AllCategories
		}
		return cmdInstallPrereq(repo, config, categories)
	case "run", "r":
		if len(categories) == 0 {
			categories = []checks.Category{checks.PrePush}
		}
		return cmdRun(repo, config, categories)
	case "run-hook":
		if categories != nil {
			return errors.New("-category can't be used with run-hook")
		}
		if flag.NArg() != 1 {
			return errors.New("run-hook is only meant to be used by hooks")
		}
		return cmdRunHook(repo, config, flag.Arg(0))
	case "version":
		if categories != nil {
			return errors.New("-category can't be used with version")
		}
		fmt.Println(version)
		return nil
	case "writeconfig", "w":
		if categories != nil {
			return errors.New("-category can't be used with writeconfig")
		}
		return cmdWriteConfig(repo, config, *configPath)
	default:
		return errors.New("unknown command, try 'help'")
	}
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "pre-commit-go: %s\n", err)
		os.Exit(1)
	}
}
