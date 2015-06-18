pre-commit-go
=============

`pre-commit-go` project includes two tools:

  - `pcg` to run checks on a Go project *on commit* and *on push* via git hooks.
    - [DESIGN.md](DESIGN.md): Designed to be correct, fast, simple,
      versatile and safe. No check ever modify any file.
    - [CI_SETUP.md](CI_SETUP.md): Native Continuous Integration service (CI)
      support.
    - [CONFIGURATION.md](CONFIGURATION.md): Configuration is easy, flexible and
      extensible.
    - [TUTORIAL.md](TUTORIAL.md): Short tutorial.
  - `covg` which is a *yet-another-coverage-tool*. It's more parallel than
    any other coverage tool and has native support for global inference.


Warning
-------

`pre-commit-go` is under heavy development. If you plan to use it as part of a
CI, please make sure to pin your version or track it closely. We'll eventually
settle and keep backward compability but the tool is not mature yet, so simply
vendor it for now.


Usage
-----

### Setup

    go get github.com/maruel/pre-commit-go/cmd/...

Use built-in help to list all options and commands:

    pcg help

Run from within a git checkout inside `$GOPATH`. This installs the git hooks
within `.git/hooks` and runs the checks in mode `pre-push`. It runs the checks
on the diff against `@{upstream}`:

    pcg


### Bypassing hook

It may become necessary to commit something known to be broken. To bypass the
pre-commit hook, use:

    git commit --no-verify  (or -n)
    git push --no-verify    (-n does something else! <3 git)


### Running coverage

    covg

You can use the `-g` flag to enable global inference, that is, coverage induced
by a unit test will work across package boundary.

#### Example coverage output

    $ ./cov -i "*.pb.go" -min 50
    common/bit_field
      coverage: 100.0% (17/17) >= 50.0%; Functions: 0 untested / 0 partially / 8 completely
    common/cache
      common/cache/cache.go:166 memory.Add            66.7% (6/9) 167,172,175
      common/cache/lru.go:79    orderedDict.popOldest 66.7% (2/3) 84
      common/cache/lru.go:200   lruDict.UnmarshalJSON 63.6% (7/11) 202,205,208,213
      common/cache/cache.go:276 disk.Add              60.0% (9/15) 277,282,291-292,295-296
      common/cache/lru.go:28    entry.UnmarshalJSON   60.0% (6/10) 31,34,41,44
      common/cache/lru.go:107   orderedDict.pushBack  40.0% (2/5) 108-110
      coverage: 84.8% (117/138) >= 50.0%; Functions: 0 untested / 6 partially / 35 completely
    common/clock
      common/clock/systemclock.go:33 systemClock.NewTimer  0.0% (0/1)
      common/clock/systemclock.go:29 systemClock.Sleep     0.0% (0/1)
      coverage: 95.2% (40/42) >= 50.0%; Functions: 2 untested / 0 partially / 18 completely


Configuration
-------------

See [Configuration](CONFIGURATION.md) for more details if you want to tweak the
default checks. The default checks are meant to be sensible, you can list them
with:

    pcg info


Continous integration support
-----------------------------

`pcg` is designed to be used as part of CI. This is [described in its
own page](CI_SETUP.md).

  - Travis: [![Build Status](https://travis-ci.org/maruel/pre-commit-go.svg?branch=master)](https://travis-ci.org/maruel/pre-commit-go)
  - CircleCI: [![Build Status](https://circleci.com/gh/maruel/pre-commit-go.svg?style=shield&circle-token=:circle-token)](https://circleci.com/gh/maruel/pre-commit-go)
  - Drone: [![Build Status](https://drone.io/github.com/maruel/pre-commit-go/status.png)](https://drone.io/github.com/maruel/pre-commit-go/latest)
  - Coveralls: [![Coverage Status](https://coveralls.io/repos/maruel/pre-commit-go/badge.svg?branch=master)](https://coveralls.io/r/maruel/pre-commit-go?branch=master)
