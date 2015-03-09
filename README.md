git pre-commit hook for Golang projects
=======================================

`pre-commit-go` runs multiple tests on a Go project to ensure code health
*before committing* via `pre-commit` git hook. It also works with on
https://travis-ci.org and publishes merged code coverage on
https://coveralls.io. It runs:

  * [Build](https://golang.org/pkg/go/build/) all directories with .go files found
  * [All tests](https://golang.org/pkg/testing/) with [coverage](https://blog.golang.org/cover)
  * [All tests](https://golang.org/pkg/testing/) with [race detector](https://blog.golang.org/race-detector)
  * [errcheck](https://github.com/kisielk/errcheck)
  * [gofmt](https://golang.org/cmd/gofmt/) and [goimports](https://godoc.org/code.google.com/p/go.tools/cmd/goimports) (redundant except for gofmt -s)
  * [govet](https://godoc.org/code.google.com/p/go.tools/cmd/vet)
  * [golint](https://github.com/golang/lint)

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
