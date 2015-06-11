Design
======

## Goals

  - Fast
  - Simple to use
  - Versatile


### Fast

  - All checks are run concurrently.
  - Checks for prerequisites is concurrent.
  - Checks are only run on the relevant code, not on the whole tree.
  - Checks are increasingly involved based on mode; pre-commit vs pre-push vs
    continuous-integration.


### Simple

  - Commit hooks are installed automatically.
  - Prerequisites are installed automatically.
  - Running it just does the right thing depending on context.
  - Easy to bypass the hook.
  - Sane defaults.


### Versatile

  - Easy to configure.
  - Yet powerful extensibility.
  - Configuration file is simple and documented by structures. No need to
    check-in the configuration file if not desired.
  - Integrated support with popular hosted CI systems. Switches automatically on
    CI mode when `CI=true` is set.
