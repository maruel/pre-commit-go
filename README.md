pre-commit-go
=============

`pre-commit-go` runs checks on a Go project *on commit* and *on push* via git
hooks.

  - It's [designed to be correct, fast, simple, versatile and safe](DESIGN.md). No
    check ever modify any file.
  - Native [Continuous Integration service (CI)](CI_SETUP.md) support.
  - [Configuration](CONFIGURATION.md) is easy, flexible and extensible.


Usage
-----

### Setup

    go get github.com/maruel/pre-commit-go

Use built-in help to list all options and commands:

    pre-commit-go help

Run from within a git checkout inside `$GOPATH`. This installs the git hooks
within `.git/hooks` and runs the checks in mode `pre-push`. It runs the checks
on the diff against `@{upstream}`:

    pre-commit-go


### Bypassing hook

It may become necessary to commit something known to be broken. To bypass the
pre-commit hook, use:

    git commit --no-verify

or shorthand `-n`


Configuration
-------------

See [Configuration](CONFIGURATION.md) for more details if you want to tweak the
default checks. The default checks are meant to be sensible, you can list them
with:

    pre-commit-go info


Continous integration support
-----------------------------

`pre-commit-go` is designed to be used as part of CI. This is [described in its
own page](CI_SETUP.md).

  - Travis: [![Build Status](https://travis-ci.org/maruel/pre-commit-go.svg?branch=master)](https://travis-ci.org/maruel/pre-commit-go)
  - CircleCI: [![Build Status](https://circleci.com/gh/maruel/pre-commit-go.svg?style=shield&circle-token=:circle-token)](https://circleci.com/gh/maruel/pre-commit-go)
  - Drone: [![Build Status](https://drone.io/github.com/maruel/pre-commit-go/status.png)](https://drone.io/github.com/maruel/pre-commit-go/latest)
  - Coveralls: [![Coverage Status](https://coveralls.io/repos/maruel/pre-commit-go/badge.svg?branch=master)](https://coveralls.io/r/maruel/pre-commit-go?branch=master)
