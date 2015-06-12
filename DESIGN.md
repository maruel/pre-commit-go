Design
======

## Goals

  - Fast
  - Correct
  - Simple to use
  - Versatile
  - Safe


### Fast

  - Between hard to implement or slow, choose fast.
  - All checks are run concurrently.
  - Lookup for prerequisites presence is concurrent.
  - Checks are only run on the relevant code, not on the whole tree.
  - Checks are increasingly involved based on mode; pre-commit vs pre-push vs
    continuous-integration.


### Correct

  - Evaluate the exact diff to determine what changed.
  - Evaluates all the import paths of all the untouched packages to discover any
    package that would be transitively (indirectly affected by a change in an
    imported package. Evaluates the dependency chain recursively. Do the same
    for tests.
      - For example, if package `./foo` is modified, `./bar` depends on `./foo`
        and `./tom` depends on `./bar`, all three packages will be tested.


### Simple to use

  - Commit hooks are installed automatically.
  - Prerequisites are installed automatically.
  - Running it just does the right thing depending on context. Switches
    automatically on CI mode when `CI=true` is set. When run directly, it diffs
    against upstream and runs checks on the local changes only. On pre-commit,
    it diffs the staged changes. On pre-push, it diffs the commits being pushed.
  - Easy to bypass the hook.
  - Sane defaults.


### Versatile

  - Easy to configure.
  - Yet powerful extensibility.
  - Configuration file is simple and documented by
    [structures](https://godoc.org/github.com/maruel/pre-commit-go/checks/definitions).
    No need to check-in the configuration file if not desired.
  - Integrated support with [popular hosted CI systems](CI_SETUP.md).


### Safe

  - No check can modify any file.
  - If any modification to the checkout is needed, it is very carefull about
    what can be done.
    - Very careful when unstaged changes are present.
  - In the normal cases (`git commit -a` and `git push` where what is pushed is
    currently checked out), the checkout is not touched.
