Configuration
=============

Modes
-----

`pre-commit-go` runs on 4 different modes:

  * `pre-commit`: it's the fast tests, e.g. running go test -short, gofmt, etc.
  * `pre-push`: the slower checks but still bearable for interactive usage.
  * `continuous-integration`: runs every checks, including the race detector.
  * `lint`: are off-by-default checks.

Default checks are meant to be sensible but it can be configured by adding a
`pre-commit-go.yaml` file, see Configuration file below.


Configuration file
------------------

`pre-commit-go` loads the on disk configuration or use the default configuration
if none is found.

In decreasing order of preference:
  - `<repo root>/.git/pre-commit-go.yml`
  - `<repo root>/pre-commit-go.yml`
  - POSIX: `~/.config/pre-commit-go.yml`; Windows: `~/pre-commit-go.yml`
  - returns the default config. You can generate it with `pre-commit-go writeconfig`

This permits to override settings of a `pre-commit-go.yml` in a repository by
storing an unversionned one in `.git`.

`pre-commit-go.yml` can be overriden on a per call basis via `-config`. If
`-config` specifies an absolute path, it is loaded directly. If it can't be
found, the default configuration is loaded.


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
