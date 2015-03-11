git pre-commit hook for Golang projects
=======================================

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
      -verbose

    Supported checks:
      Native ones that only depends on the stdlib:
        - go build
        - go test
        - gofmt -s
      Checks that have prerequisites (which will be automatically installed):
        - errcheck
        - goimports
        - golint
        - go tool vet
        - go test -cover

    No check ever modify any file.

`pre-commit-go` runs multiple checks on a Go project *before committing* via
`pre-commit` git hook. Native checks:

  * [go build](https://golang.org/pkg/go/build/) all directories with .go files found
  * [go test](https://golang.org/pkg/testing/) by default with [race detector](https://blog.golang.org/race-detector)
  * [gofmt](https://golang.org/cmd/gofmt/) and [goimports](https://godoc.org/code.google.com/p/go.tools/cmd/goimports) (redundant except for gofmt -s)
  * [errcheck](https://github.com/kisielk/errcheck)
  * [goimports](https://golang.org/x/tools/cmd/goimports)
  * [golint](https://github.com/golang/lint)
  * [govet](https://golang.org/x/tools/cmd/vet)
  * [go test -cover](https://golang.org/pkg/testing/) with [coverage](https://blog.golang.org/cover)


### Getting it

    go get github.com/maruel/pre-commit-go


### Installing the hook and running checks

    pre-commit-go

from within a git checkout inside `$GOPATH`.


### Help page

    pre-commit-go --help

If you want to bypass the pre-commit hook due to known breakage, use:

    git commit --no-verify


Travis & Coveralls integration
---------------------------------

Post push CI (continuous integration) works with https://travis-ci.org and
https://coveralls.io.

   1. Visit https://travis-ci.org and connect your github account (or whatever
      git host provider) to travis.
   2. Do the same via https://coveralls.io.
   3. add a `.travis.yml` file to your repository and push it.

Sample `.travis.yml`:

    sudo: false
    language: go
    go:
    - 1.4
    before_install:
      - go get github.com/maruel/pre-commit-go
    script:
      - pre-commit-go
