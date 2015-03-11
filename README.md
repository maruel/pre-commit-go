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
        - [go build](https://golang.org/pkg/go/build/)
        - go test
        - gofmt -s
      Checks that have prerequisites (which will be automatically installed):
        - errcheck
        - goimports
        - golint
        - go tool vet
        - go test -cover

    No check ever modify any file.

`pre-commit-go` runs multiple checks on a Go project to ensure code health
*before committing* via `pre-commit` git hook. It also works with on
https://travis-ci.org and publishes merged code coverage on
https://coveralls.io. It runs:

  * [go build](https://golang.org/pkg/go/build/) all directories with .go files found
  * [go test](https://golang.org/pkg/testing/) by default with [race detector](https://blog.golang.org/race-detector)
  * [gofmt](https://golang.org/cmd/gofmt/) and [goimports](https://godoc.org/code.google.com/p/go.tools/cmd/goimports) (redundant except for gofmt -s)
  * [errcheck](https://github.com/kisielk/errcheck)
  * [goimports](https://golang.org/x/tools/cmd/goimports)
  * [golint](https://github.com/golang/lint)
  * [govet](https://golang.org/x/tools/cmd/vet)
  * [go test -cover](https://golang.org/pkg/testing/) with [coverage](https://blog.golang.org/cover)

Getting it:

    go get github.com/maruel/pre-commit-go


Installing the `pre-commit` hook and running checks:

    pre-commit-go

from within a git checkout inside `$GOPATH`.

Help page:

    pre-commit-go --help

If you want to bypass the pre-commit hook due to known breakage, use:

    git commit --no-verify


Travis & Coveralls post push hook
---------------------------------

Post push CI (continuous integration) works with travis-ci.org and coveralls.io.

First, visit https://travis-ci.org and connect your github account (or whatever
git host provider) to travis.

Second, do the same via https://coveralls.io.

Third, add a file to your repository:

    # TODO(maruel): Document how to create proper .travis.yml.
    git add .travis.yml
    git commit -m "Added .travis.yml"
