git pre-commit hook for Golang projects
=======================================

    $ ./pre-commit-go help
    pre-commit-go: runs pre-commit checks on Go projects.

    Supported commands are:
      help        - this page
      install     - install the git commit hook as .git/hooks/pre-commit
      prereq      - install prerequisites, e.g.: errcheck, golint, goimports, govet,
                    etc as applicable for the enabled checks.
      run         - run all enabled checks
      writeconfig - write (or rewrite) a pre-commit-go.json

    When executed without command, it does the equivalent of 'prereq', 'install'
    then 'run'.

    Supported flags are:
      -config="pre-commit-go.json": file name of the config to load
      -level=1: runlevel, between 0 and 3; the higher, the more tests are run
      -verbose=false: enables verbose logging output

    Supported checks and their runlevel:
      Native checks that only depends on the stdlib:
        - build        1 : builds all packages that do not contain tests, usually all directories with package 'main'
        - gofmt        1 : enforces all .go sources are formatted with 'gofmt -s'
        - test         1 : runs all tests, potentially multiple times (with race detector, with different tags, etc)

      Checks that have prerequisites (which will be automatically installed):
        - errcheck     2 : enforces all calls returning an error are checked using tool 'errcheck'
        - goimports    2 : enforces all .go sources are formatted with 'goimports'
        - golint       3 : enforces all .go sources passes golint
        - govet        3 : enforces all .go sources passes go tool vet
        - testcoverage 2 : enforces minimum test coverage on all packages that are not 'main'

    No check ever modify any file.

`pre-commit-go` runs multiple checks on a Go project *before committing* via
`pre-commit` git hook. Native checks:

  * [go build](https://golang.org/pkg/go/build/) all directories with .go files found
  * [go test](https://golang.org/pkg/testing/) by default with [race detector](https://blog.golang.org/race-detector)
  * [gofmt](https://golang.org/cmd/gofmt/) and [goimports](https://godoc.org/code.google.com/p/go.tools/cmd/goimports) (redundant except for gofmt -s)
  * [errcheck](https://github.com/kisielk/errcheck)
  * [goimports](https://golang.org/x/tools/cmd/goimports)
  * [golint](https://github.com/golang/lint)
  * [govet (go tool vet)](https://golang.org/x/tools/cmd/vet)
  * [go test -cover](https://golang.org/pkg/testing/) with [coverage](https://blog.golang.org/cover)

See all the checks documentation at [![GoDoc](https://godoc.org/github.com/maruel/pre-commit-go/checks?status.svg)](https://godoc.org/github.com/maruel/pre-commit-go/checks)


### Getting it

    go get github.com/maruel/pre-commit-go


### Installing the hook and running checks

From within a git checkout inside `$GOPATH`:

    pre-commit-go


### Bypassing hook

To bypass the pre-commit hook due to known breakage, use:

    git commit --no-verify


Travis & Coveralls integration
---------------------------------

Post push CI (continuous integration) works with Travis and Coveralls. This
runs the checks on pull requests automatically! This also works with
github organizations.

   1. Visit https://travis-ci.org and connect your github account (or whatever
      git host provider) to Travis. Enable your repository.
   2. Do the same via https://coveralls.io.
   3. Add a `.travis.yml` file to your repository and push it.

Sample `.travis.yml`:

    sudo: false
    language: go
    go:
    - 1.4
    before_install:
      - go get github.com/maruel/pre-commit-go
    script:
      - pre-commit-go run -level 2
