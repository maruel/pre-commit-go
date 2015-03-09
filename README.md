git pre-commit hook for Golang projects
=======================================

`pre-commit-go` runs multiple tests on a Go project to ensure code health
*before committing*. It also helps to run automated testing on https://travis-ci.org and
publish code coverage on https://coveralls.io.

It is designed to be called on pre-commit so that the committed code is clean.
It runs:

  * [Build](https://golang.org/pkg/go/build/) all directories with .go files found
  * [All tests](https://golang.org/pkg/testing/) with [coverage](https://blog.golang.org/cover)
  * [All tests](https://golang.org/pkg/testing/) with [race detector](https://blog.golang.org/race-detector)
  * [errcheck](https://github.com/kisielk/errcheck)
  * [gofmt](https://golang.org/cmd/gofmt/) and [goimports](https://godoc.org/code.google.com/p/go.tools/cmd/goimports) (redundant except for gofmt -s)
  * [govet](https://godoc.org/code.google.com/p/go.tools/cmd/vet)
  * [golint](https://github.com/golang/lint)

To get it; use:

    go get github.com/maruel/pre-commit-go


To both install the `pre-commit` hook and run checks, use:

    pre-commit-go

from within a git checkout inside `$GOPATH`.


Travis & Coveralls post push hook
---------------------------------

Post push CI works with travis-ci.org and coveralls.io. Do:

    # TODO(maruel): Document how to create proper .travis.yml.
    git add .travis.yml
    git commit -m "Added .travis.yml"


Visit https://travis-ci.org and connect your github account (or whatever git
host provider) to travis. Then do the same via https://coveralls.io.
