// Copyright 2016 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// pcg: runs pre-commit checks on Go projects.
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
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/maruel/pre-commit-go/checks"
	"github.com/maruel/pre-commit-go/internal"
	"github.com/maruel/pre-commit-go/scm"
	"gopkg.in/yaml.v2"
)

// Globals

// Bump when the CLI, configuration file format or behavior change in any
// significant way. This will make files written by this version backward
// incompatible, forcing downstream users to update their pre-commit-go
// version.
const version = "0.4.7"

const hookContent = `#!/bin/sh
# AUTOGENERATED BY pcg.
#
# For more information, run:
#   pcg help
#
# or visit https://github.com/maruel/pre-commit-go

set -e
pcg run-hook %s
`

const gitNilCommit = "0000000000000000000000000000000000000000"

const helpModes = "Supported modes (with shortcut names):\n- pre-commit / fast / pc\n- pre-push / slow / pp  (default)\n- continous-integration / full / ci\n- lint\n- all: includes both continuous-integration and lint"

// http://git-scm.com/docs/githooks#_pre_push
var rePrePush = regexp.MustCompile("^(.+?) ([0-9a-f]{40}) (.+?) ([0-9a-f]{40})$")

var helpText = template.Must(template.New("help").Parse(`pcg: runs pre-commit checks on Go projects, fast.

Supported commands are:
  help        - this page
  prereq      - installs prerequisites, e.g.: errcheck, golint, goimports,
                govet, etc as applicable for the enabled checks
  info        - prints the current configuration used
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

var parsedVersion []int

// Runtime Options.
type application struct {
	config        *checks.Config
	maxConcurrent int
}

// Utils.

func init() {
	var err error
	parsedVersion, err = parseVersion(version)
	if err != nil {
		panic(err)
	}
}

// parseVersion converts a "1.2.3" string into []int{1,2,3}.
func parseVersion(v string) ([]int, error) {
	out := []int{}
	for _, i := range strings.Split(v, ".") {
		v, err := strconv.ParseInt(i, 10, 32)
		if err != nil {
			return nil, err
		}
		out = append(out, int(v))
	}
	return out, nil
}

// loadConfigFile returns a Config with defaults set then loads the config from
// file "pathname".
func loadConfigFile(pathname string) *checks.Config {
	content, err := ioutil.ReadFile(pathname)
	if err != nil {
		return nil
	}
	config := &checks.Config{}
	if err := yaml.Unmarshal(content, config); err != nil {
		// Log but ignore the error, recreate a new config instance.
		log.Printf("failed to parse %s: %s", pathname, err)
		return nil
	}
	configVersion, err := parseVersion(config.MinVersion)
	if err != nil {
		log.Printf("invalid version %s", config.MinVersion)
	}
	for i, v := range configVersion {
		if len(parsedVersion) <= i {
			if v == 0 {
				// 3.0 == 3.0.0
				continue
			}
			log.Printf("requires newer version %s", config.MinVersion)
			return nil
		}
		if parsedVersion[i] > v {
			break
		}
		if parsedVersion[i] < v {
			log.Printf("requires newer version %s", config.MinVersion)
			return nil
		}
	}
	return config
}

// loadConfig loads the on disk configuration or use the default configuration
// if none is found. See CONFIGURATION.md for the logic.
func loadConfig(repo scm.ReadOnlyRepo, path string) (string, *checks.Config) {
	if filepath.IsAbs(path) {
		if config := loadConfigFile(path); config != nil {
			return path, config
		}
	} else {
		// <repo root>/.git/<path>
		if scmDir, err := repo.ScmDir(); err == nil {
			file := filepath.Join(scmDir, path)
			if config := loadConfigFile(file); config != nil {
				return file, config
			}
		}

		// <repo root>/<path>
		file := filepath.Join(repo.Root(), path)
		if config := loadConfigFile(file); config != nil {
			return file, config
		}

		if user, err := user.Current(); err == nil && user.HomeDir != "" {
			if runtime.GOOS == "windows" {
				// ~/<path>
				file = filepath.Join(user.HomeDir, path)
			} else {
				// ~/.config/<path>
				file = filepath.Join(user.HomeDir, ".config", path)
			}
			if config := loadConfigFile(file); config != nil {
				return file, config
			}
		}
	}
	return "<N/A>", checks.New(version)
}

func callRun(check checks.Check, change scm.Change, options *checks.Options) (time.Duration, error) {
	start := time.Now()
	err := check.Run(change, options)
	return time.Now().Sub(start), err
}

func (a *application) runChecks(change scm.Change, modes []checks.Mode, prereqReady *sync.WaitGroup) error {
	enabledChecks, options := a.config.EnabledChecks(modes)
	log.Printf("mode: %s; %d checks; %d max seconds allowed", modes, len(enabledChecks), options.MaxDuration)
	if change == nil {
		log.Printf("no change")
		return nil
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(enabledChecks))
	warnings := make(chan error, len(enabledChecks))
	start := time.Now()
	for _, c := range enabledChecks {
		wg.Add(1)
		go func(check checks.Check) {
			defer wg.Done()
			if len(check.GetPrerequisites()) != 0 {
				// If this check has prerequisites, wait for all prerequisites to be
				// checked for presence.
				prereqReady.Wait()
			}
			log.Printf("%s...", check.GetName())
			duration, err := callRun(check, change, options)
			if err != nil {
				log.Printf("... %s in %1.2fs FAILED\n%s", check.GetName(), duration.Seconds(), err)
				errs <- err
				return
			}
			log.Printf("... %s in %1.2fs", check.GetName(), duration.Seconds())
			// A check that took too long is a check that failed.
			max := time.Duration(options.MaxDuration) * time.Second
			if duration > max {
				warnings <- fmt.Errorf("check %s took %1.2fs -> IT IS TOO SLOW (limit: %s)", check.GetName(), duration.Seconds(), max)
			}
		}(c)
	}
	wg.Wait()

	var err error
	for {
		select {
		case err = <-errs:
			fmt.Printf("%s\n", err)
		case warning := <-warnings:
			fmt.Printf("warning: %s\n", warning)
		default:
			if err != nil {
				duration := time.Now().Sub(start)
				return fmt.Errorf("checks failed in %1.2fs", duration.Seconds())
			}
			return err
		}
	}
}

func (a *application) runPreCommit(repo scm.Repo) error {
	// First, stash index and work dir, keeping only the to-be-committed changes
	// in the working directory.
	// TODO(maruel): When running for an git commit --amend run, use HEAD~1.
	stashed, err := repo.Stash()
	if err != nil {
		return err
	}
	// Run the checks.
	var change scm.Change
	change, err = repo.Between(scm.Current, scm.Head, a.config.IgnorePatterns)
	if change != nil {
		err = a.runChecks(change, []checks.Mode{checks.PreCommit}, &sync.WaitGroup{})
	}
	// If stashed is false, everything was in the index so no stashing was needed.
	if stashed {
		if err2 := repo.Restore(); err == nil {
			err = err2
		}
	}
	return err
}

func (a *application) runPrePush(repo scm.Repo) (err error) {
	previous := scm.Head
	// Will be "" if the current checkout was detached.
	previousRef := repo.Ref(scm.Head)
	curr := previous
	stashed := false
	defer func() {
		if curr != previous {
			p := previousRef
			if p == "" {
				p = string(previous)
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
	triedToStash := false
	for {
		if line, err = bio.ReadString('\n'); err != nil {
			break
		}
		matches := rePrePush.FindStringSubmatch(line[:len(line)-1])
		if len(matches) != 5 {
			return fmt.Errorf("unexpected stdin for pre-push: %q", line)
		}
		from := scm.Commit(matches[4])
		to := scm.Commit(matches[2])
		if to == gitNilCommit {
			// It's being deleted.
			continue
		}
		if to != curr {
			// Stash, checkout, run tests.
			if !triedToStash {
				// Only try to stash once.
				triedToStash = true
				if stashed, err = repo.Stash(); err != nil {
					return
				}
			}
			curr = to
			if err = repo.Checkout(string(to)); err != nil {
				return
			}
		}
		if from == gitNilCommit {
			from = scm.Initial
		}
		change, err := repo.Between(to, from, a.config.IgnorePatterns)
		if err != nil {
			return err
		}
		if err = a.runChecks(change, []checks.Mode{checks.PrePush}, &sync.WaitGroup{}); err != nil {
			return err
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func processModes(modeFlag string) ([]checks.Mode, error) {
	if len(modeFlag) == 0 {
		return nil, nil
	}
	var modes []checks.Mode
	for _, p := range strings.Split(modeFlag, ",") {
		if len(p) != 0 {
			switch p {
			case "all":
				modes = append(modes, checks.ContinuousIntegration, checks.Lint)
			case string(checks.PreCommit), "fast", "pc":
				modes = append(modes, checks.PreCommit)
			case string(checks.PrePush), "slow", "pp":
				modes = append(modes, checks.PrePush)
			case string(checks.ContinuousIntegration), "full", "ci":
				modes = append(modes, checks.ContinuousIntegration)
			case string(checks.Lint):
				modes = append(modes, checks.Lint)
			default:
				return nil, fmt.Errorf("invalid mode \"%s\"\n\n%s", p, helpModes)
			}
		}
	}
	return modes, nil
}

type sortedChecks []checks.Check

func (s sortedChecks) Len() int           { return len(s) }
func (s sortedChecks) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sortedChecks) Less(i, j int) bool { return s[i].GetName() < s[j].GetName() }

// Commands.

func (a *application) cmdHelp(usage string) error {
	s := &struct {
		Usage        string
		Max          int
		NativeChecks sortedChecks
		OtherChecks  sortedChecks
	}{
		usage,
		0,
		sortedChecks{},
		sortedChecks{},
	}
	for name, factory := range checks.KnownChecks {
		if v := len(name); v > s.Max {
			s.Max = v
		}
		c := factory()
		if len(c.GetPrerequisites()) == 0 {
			s.NativeChecks = append(s.NativeChecks, c)
		} else {
			s.OtherChecks = append(s.OtherChecks, c)
		}
	}
	sort.Sort(s.NativeChecks)
	sort.Sort(s.OtherChecks)
	return helpText.Execute(os.Stdout, s)
}

// cmdInfo displays the current configuration used.
func (a *application) cmdInfo(repo scm.ReadOnlyRepo, modes []checks.Mode, configPath string) error {
	fmt.Printf("File: %s\n", configPath)
	fmt.Printf("Repo: %s\n", repo.Root())

	fmt.Printf("MinVersion: %s\n", a.config.MinVersion)
	content, err := yaml.Marshal(a.config.IgnorePatterns)
	if err != nil {
		return err
	}
	fmt.Printf("IgnorePatterns:\n%s", content)

	if len(modes) == 0 {
		modes = checks.AllModes
	}
	for _, mode := range modes {
		settings := a.config.Modes[mode]
		maxLen := 0
		for _, checks := range settings.Checks {
			for _, check := range checks {
				if l := len(check.GetName()); l > maxLen {
					maxLen = l
				}
			}
		}
		fmt.Printf("\n%s:\n  %-*s %d seconds\n", mode, maxLen+1, "Limit:", settings.Options.MaxDuration)
		for _, checks := range settings.Checks {
			for _, check := range checks {
				name := check.GetName()
				fmt.Printf("  %s:%s %s\n", name, strings.Repeat(" ", maxLen-len(name)), check.GetDescription())
				content, err := yaml.Marshal(check)
				if err != nil {
					return err
				}
				options := strings.TrimSpace(string(content))
				if options == "{}" {
					// It means there's no options.
					options = "<no option>"
				}
				lines := strings.Join(strings.Split(options, "\n"), "\n    ")
				fmt.Printf("    %s\n", lines)
			}
		}
	}
	return nil
}

// cmdInstallPrereq installs all the packages needed to run the enabled checks.
func (a *application) cmdInstallPrereq(repo scm.ReadOnlyRepo, modes []checks.Mode, noUpdate bool) error {
	var wg sync.WaitGroup
	enabledChecks, _ := a.config.EnabledChecks(modes)
	number := 0
	c := make(chan string, len(enabledChecks))
	for _, check := range enabledChecks {
		for _, p := range check.GetPrerequisites() {
			number++
			wg.Add(1)
			go func(prereq checks.CheckPrerequisite) {
				defer wg.Done()
				if !prereq.IsPresent() {
					c <- prereq.URL
				}
			}(p)
		}
	}
	wg.Wait()
	log.Printf("Checked for %d prerequisites", number)
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
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	sort.Strings(urls)
	if len(urls) != 0 {
		if noUpdate {
			out := "-n is specified but prerequites are missing:\n"
			for _, url := range urls {
				out += "  " + url + "\n"
			}
			return errors.New(out)
		}
		fmt.Printf("Installing:\n")
		for _, url := range urls {
			fmt.Printf("  %s\n", url)
		}

		out, _, err := internal.Capture(wd, nil, append([]string{"go", "get"}, urls...)...)
		if len(out) != 0 {
			return fmt.Errorf("prerequisites installation failed: %s", out)
		}
		if err != nil {
			return fmt.Errorf("prerequisites installation failed: %s", err)
		}
	}
	log.Printf("Prerequisites installation succeeded")
	return nil
}

// cmdInstall first calls cmdInstallPrereq() then install the
// .git/hooks/pre-commit and pre-push hooks.
//
// Silently ignore installing the hooks when running under a CI. In
// particular, circleci.com doesn't create the directory .git/hooks.
func (a *application) cmdInstall(repo scm.ReadOnlyRepo, modes []checks.Mode, noUpdate bool, prereqReady *sync.WaitGroup) (err error) {
	errCh := make(chan error, 1)
	go func() {
		defer prereqReady.Done()
		errCh <- a.cmdInstallPrereq(repo, modes, noUpdate)
	}()

	defer func() {
		if err2 := <-errCh; err == nil {
			err = err2
		}
	}()

	if checks.IsContinuousIntegration() {
		log.Printf("Running under CI; not installing hooks")
		return nil
	}
	log.Printf("Installing hooks")
	hookDir, err2 := repo.HookPath()
	if err2 != nil {
		return err2
	}
	for _, t := range []string{"pre-commit", "pre-push"} {
		// Always remove hook first if it exists, in case it's a symlink.
		p := filepath.Join(hookDir, t)
		_ = os.Remove(p)
		if err = ioutil.WriteFile(p, []byte(fmt.Sprintf(hookContent, t)), 0777); err != nil {
			return err
		}
	}
	log.Printf("Installation done")
	return nil
}

// cmdRun runs all the enabled checks.
func (a *application) cmdRun(repo scm.ReadOnlyRepo, modes []checks.Mode, against string, prereqReady *sync.WaitGroup) error {
	var old scm.Commit
	if against != "" {
		if old = repo.Eval(against); old == scm.Invalid {
			return errors.New("invalid commit 'against'")
		}
	} else {
		if old = repo.Eval(string(scm.Upstream)); old == scm.Invalid {
			return errors.New("no upstream")
		}
	}
	change, err := repo.Between(scm.Current, old, a.config.IgnorePatterns)
	if err != nil {
		return err
	}
	return a.runChecks(change, modes, prereqReady)
}

// cmdRunHook runs the checks in a git repository.
//
// Use a precise "stash, run checks, unstash" to ensure that the check is
// properly run on the data in the index.
func (a *application) cmdRunHook(repo scm.Repo, mode string, noUpdate bool) error {
	switch checks.Mode(mode) {
	case checks.PreCommit:
		return a.runPreCommit(repo)

	case checks.PrePush:
		return a.runPrePush(repo)

	case checks.ContinuousIntegration:
		// Always runs all tests on CI.
		change, err := repo.Between(scm.Current, scm.Initial, a.config.IgnorePatterns)
		if err != nil {
			return err
		}
		mode := []checks.Mode{checks.ContinuousIntegration}

		// This is a special case, some users want reproducible builds and in this
		// case they do not want any external reference and want to enforce
		// noUpdate, but many people may not care (yet). So default to fetching but
		// it can be overriden.
		var prereqReady sync.WaitGroup
		errCh := make(chan error, 1)
		prereqReady.Add(1)
		go func() {
			defer prereqReady.Done()
			errCh <- a.cmdInstallPrereq(repo, mode, noUpdate)
		}()
		err = a.runChecks(change, mode, &prereqReady)
		if err2 := <-errCh; err2 != nil {
			return err2
		}
		return err

	default:
		return errors.New("unsupported hook type for run-hook")
	}
}

func (a *application) cmdWriteConfig(repo scm.ReadOnlyRepo, configPath string) error {
	a.config.MinVersion = version
	content, err := yaml.Marshal(a.config)
	if err != nil {
		return fmt.Errorf("internal error when marshaling config: %s", err)
	}
	_ = os.Remove(configPath)
	return ioutil.WriteFile(configPath, append([]byte(yamlHeader), content...), 0666)
}

// mainImpl implements pcg.
func mainImpl() error {
	a := application{}

	exec, args := os.Args[0], os.Args[1:]
	var commands, flags []string
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flags = args[i:]
			break
		}
		commands = append(commands, arg)
	}

	if len(commands) == 0 {
		if checks.IsContinuousIntegration() {
			commands = []string{"run-hook", "continuous-integration"}
		} else {
			commands = []string{"installrun"}
		}
	}

	fs := flag.NewFlagSet(exec, flag.ExitOnError)
	fs.Usage = func() {
		b := &bytes.Buffer{}
		fs.SetOutput(b)
		fs.PrintDefaults()
		_ = a.cmdHelp(b.String())
	}
	verboseFlag := fs.Bool("v", checks.IsContinuousIntegration() || os.Getenv("VERBOSE") != "", "enables verbose logging output")
	allFlag := fs.Bool("a", false, "runs checks as if all files had been modified")
	againstFlag := fs.String("r", "", "runs checks on files modified since this revision, as evaluated by your scm repo")
	noUpdateFlag := fs.Bool("n", false, "disallow using go get even if a prerequisite is missing; bail out instead")
	configPathFlag := fs.String("c", "pre-commit-go.yml", "file name of the config to load")
	modeFlag := fs.String("m", "", "comma separated list of modes to process; default depends on the command")
	fs.IntVar(&a.maxConcurrent, "C", 0, "maximum number of concurrent processes")
	if err := fs.Parse(flags); err != nil {
		return err
	}

	if *allFlag {
		if *againstFlag != "" {
			return errors.New("-a can't be used with -r")
		}
		*againstFlag = string(scm.Initial)
	}

	log.SetFlags(log.Lmicroseconds)
	if !*verboseFlag {
		log.SetOutput(ioutil.Discard)
	}

	modes, err := processModes(*modeFlag)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := scm.GetRepo(cwd, "")
	if err != nil {
		return err
	}

	var configPath string
	configPath, a.config = loadConfig(repo, *configPathFlag)
	log.Printf("config: %s", configPath)
	if a.maxConcurrent > 0 {
		log.Printf("using %d maximum concurrent goroutines", a.maxConcurrent)
		a.config.MaxConcurrent = a.maxConcurrent
	}

	switch cmd := commands[0]; cmd {
	case "help", "-help", "-h":
		cmd = "help"
		if *allFlag != false {
			return fmt.Errorf("-a can't be used with %s", cmd)
		}
		if *againstFlag != "" {
			return fmt.Errorf("-r can't be used with %s", cmd)
		}
		if *noUpdateFlag != false {
			return fmt.Errorf("-n can't be used with %s", cmd)
		}
		if *configPathFlag != "pre-commit-go.yml" {
			return fmt.Errorf("-m can't be used with %s", cmd)
		}
		if *modeFlag != "" {
			return fmt.Errorf("-m can't be used with %s", cmd)
		}
		b := &bytes.Buffer{}
		fs.SetOutput(b)
		fs.PrintDefaults()
		return a.cmdHelp(b.String())

	case "info":
		if *allFlag != false {
			return fmt.Errorf("-a can't be used with %s", cmd)
		}
		if *againstFlag != "" {
			return fmt.Errorf("-r can't be used with %s", cmd)
		}
		if *noUpdateFlag != false {
			return fmt.Errorf("-n can't be used with %s", cmd)
		}
		return a.cmdInfo(repo, modes, configPath)

	case "install", "i":
		cmd = "install"
		if *allFlag != false {
			return fmt.Errorf("-a can't be used with %s", cmd)
		}
		if *againstFlag != "" {
			return fmt.Errorf("-r can't be used with %s", cmd)
		}
		if len(modes) == 0 {
			modes = checks.AllModes
		}
		var prereqReady sync.WaitGroup
		prereqReady.Add(1)
		return a.cmdInstall(repo, modes, *noUpdateFlag, &prereqReady)

	case "installrun":
		if len(modes) == 0 {
			modes = []checks.Mode{checks.PrePush}
		}
		// Start running all checks that do not have a prerequisite before
		// installation is completed.
		var prereqReady sync.WaitGroup
		prereqReady.Add(1)
		errCh := make(chan error, 1)
		go func() {
			errCh <- a.cmdInstall(repo, modes, *noUpdateFlag, &prereqReady)
		}()
		err := a.cmdRun(repo, modes, *againstFlag, &prereqReady)
		if err2 := <-errCh; err2 != nil {
			return err2
		}
		return err

	case "prereq", "p":
		cmd = "prereq"
		if *allFlag != false {
			return fmt.Errorf("-a can't be used with %s", cmd)
		}
		if *againstFlag != "" {
			return fmt.Errorf("-r can't be used with %s", cmd)
		}
		if len(modes) == 0 {
			modes = checks.AllModes
		}
		return a.cmdInstallPrereq(repo, modes, *noUpdateFlag)

	case "run", "r":
		cmd = "run"
		if *noUpdateFlag != false {
			return fmt.Errorf("-n can't be used with %s", cmd)
		}
		if len(modes) == 0 {
			modes = []checks.Mode{checks.PrePush}
		}
		return a.cmdRun(repo, modes, *againstFlag, &sync.WaitGroup{})

	case "run-hook":
		if modes != nil {
			return fmt.Errorf("-m can't be used with %s", cmd)
		}
		if *allFlag != false {
			return fmt.Errorf("-a can't be used with %s", cmd)
		}
		if *againstFlag != "" {
			return fmt.Errorf("-r can't be used with %s", cmd)
		}

		if len(commands) < 2 {
			return errors.New("run-hook is only meant to be used by hooks")
		}
		return a.cmdRunHook(repo, commands[1], *noUpdateFlag)

	case "version":
		if modes != nil {
			return fmt.Errorf("-m can't be used with %s", cmd)
		}
		if *allFlag != false {
			return fmt.Errorf("-a can't be used with %s", cmd)
		}
		if *againstFlag != "" {
			return fmt.Errorf("-r can't be used with %s", cmd)
		}
		if *noUpdateFlag != false {
			return fmt.Errorf("-n can't be used with %s", cmd)
		}
		fmt.Println(version)
		return nil

	case "writeconfig", "w":
		if modes != nil {
			return fmt.Errorf("-m can't be used with %s", cmd)
		}
		if *allFlag != false {
			return fmt.Errorf("-a can't be used with %s", cmd)
		}
		if *againstFlag != "" {
			return fmt.Errorf("-r can't be used with %s", cmd)
		}
		// Note that in that case, configPath is ignored and not overritten.
		return a.cmdWriteConfig(repo, *configPathFlag)

	default:
		return fmt.Errorf("unknown command %q, try 'help'", cmd)
	}
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "pcg: %s\n", err)
		os.Exit(1)
	}
}
