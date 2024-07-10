## Semgrep

CodeQL like OSS linter

Example usage:

```
jobs:
  semgrep:
    uses: TykTechnologies/github-actions/.github/workflows/semgrep.yaml@main
```

Usage: unknown;

### Core: We are looking at semgrep for refactorings.

If you'd like to use semgrep:

- reach out to @titpetric,
- https://blog.semgrep-ci.com/
- https://github.com/TykTechnologies/exp/tree/main/lsc
- https://github.com/TykTechnologies/exp/actions/workflows/semgrep.yml

The current state allows to automate refactorings with semgrep, by using
github actions automation to open up PR's against target repositories.

Example outputs:

- https://github.com/TykTechnologies/tyk/pull/6380
- https://github.com/TykTechnologies/tyk-analytics/pull/4051

We experience several problems where semgrep could be used more extensively:

- code cleanups to enforce consistent style
- large scale refactorings
- ensuring code style compliance with new contributions
- detecting bugs based on our own rules/bugs occuring
