pre-commit-go
=============

`pre-commit-go` runs multiple checks on a Go project *on commit* via
`pre-commit` git hook and *on push* via `pre-push` git hook. It's designed to be
simple and *fast*. Everything is run concurrently. It also includes linting
support and Continuous Integration service (CI) support. No check ever modify
any file.

[![Build Status](https://travis-ci.org/maruel/pre-commit-go.svg?branch=master)](https://travis-ci.org/maruel/pre-commit-go)


Modes
-----

`pre-commit-go` runs on 4 different modes:

  * `pre-commit`: it's the fast tests, e.g. running go test -short
  * `pre-push`: the slower checks but still bearable for interactive usage.
  * `continuous-integration`: runs every checks, including the race detector.
  * `lint`: are off-by-default checks.

Default checks are meant to be sensible but it can be configured by adding a
[pre-commit-go.yml](https://github.com/maruel/pre-commit-go/blob/master/pre-commit-go.yml)
in your git checkout root directory. If you don't want to pollute your git
repository with yml files, put it at `.git/pre-commit-go.yml`.


Checks
------

Checks documentation:
[![GoDoc](https://godoc.org/github.com/maruel/pre-commit-go/checks/definitions?status.svg)](https://godoc.org/github.com/maruel/pre-commit-go/checks/definitions)


### Native checks

  * [go build](https://golang.org/pkg/go/build/) all directories with .go files
    found
  * [go test -race](https://golang.org/pkg/testing/) by default with [race
    detector](https://blog.golang.org/race-detector)
  * [go test -cover](https://golang.org/pkg/testing/) with
    [coverage](https://blog.golang.org/cover)
  * [gofmt](https://golang.org/cmd/gofmt/), especially for the -s flag.
  * [goimports](https://golang.org/x/tools/cmd/goimports)
  * [errcheck](https://github.com/kisielk/errcheck)
  * [golint](https://github.com/golang/lint)
  * [govet (go tool vet)](https://golang.org/x/tools/cmd/vet)


### Custom check

A custom check can be defined by adding a `custom` check in one of the modes.
Here's an example running `sample-pre-commit-go-custom-check` on the tree in
mode continuous-integration:

```yaml
modes:
  continous-integration:
    checks:
    - check_type: custom
      display_name: sample-pre-commit-go-custom-check
      description: runs the check sample-pre-commit-go-custom-check on this repository
      command:
      - sample-pre-commit-go-custom-check
      - check
      check_exit_code: true
      prerequisites:
      - help_command:
        - sample-pre-commit-go-custom-check
        - -help
        expected_exit_code: 2
        url: github.com/maruel/pre-commit-go/samples/sample-pre-commit-go-custom-check
```


Usage
-----

### Getting it

    go get github.com/maruel/pre-commit-go


### Installing the git hooks and running checks

Run from within a git checkout inside `$GOPATH`:

    pre-commit-go

Then use built-in help:

    pre-commit-go help


### Bypassing hook

It may become necessary to commit something known to be broken. To bypass the
pre-commit hook, use:

    git commit --no-verify

or shorthand `-n`


Continous integration support
-----------------------------

### travis-ci.org

Post push CI (continuous integration) works with Travis. This
runs the checks on pull requests automatically! This also works with
github organizations.

   1. Visit https://travis-ci.org and connect your github account (or whatever
      git host provider) to Travis. Enable your repository.
   2. Copy
      [sample/travis.yml](https://github.com/maruel/pre-commit-go/blob/master/sample/travis.yml)
      as `.travis.yml` in your repository and push it.


### coveralls.io

Integrate with travis-ci first, then visit https://coveralls.io and enable your
repository. That's all automatic.


### drone.io

TODO(maruel): Add explanation.
