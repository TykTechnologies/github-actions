# Re-usable github actions

Collection of shared github actions and workflows which are used in our org.

# Composite actions

## PR Checkout

The checkout PR action will fetch only the commits that belong to the PR.
This is required for various code analysis tooling, including sonarcloud.

Example usage:

```yaml
jobs:
  golangci-lint:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout PR
        uses: TykTechnologies/github-actions/.github/actions/checkout-pr@main
```

The main use case behind this is to make sure the HEAD and the current PR
state can be compared, and that we don't fetch the full git history for
the checkout. This supports some of our custom actions like `godoc`.

Supports: godoc, sonarcloud, dashboard (bindata size).

Adoption: gateway, dashboard, reuse in shared CI workflows.

Source: [/.github/actions/checkout-pr/action.yml](/.github/actions/checkout-pr/action.yml)

## Github to slack

Maps github email with slack user, based on a key value map. Maps needs to be mantained manually.

Source: [/.github/actions/github-to-slack/action.yaml](/.github/actions/github-to-slack/action.yaml)

## Calculate tests tags

Calculates corresponding CI image tags based on github events for a group of tyk repositories

Source: [/.github/actions/latest-versions/action.yaml](/.github/actions/latest-versions/action.yaml)

# Reusable workflows

## CI tooling

We build a docker image from the CI pipeline in this repository that
builds and installs all the CI tooling needed for the test pipelines.

Providing the docker image avoids continous compilation of the tools from
using `go install` or `go get`, decreasing resource usage on GitHub
actions.

All the tools are built using a recent go version and `CGO_ENABLED=0`,
enabling reuse for old releases. It's still possible to version the
tooling against releases either inside the image, or by creating new
versions of the docker image in the future.

The images built are:

- `tykio/ci-tools:latest`.

The image is rebuilt weekly and on triggers from `exp/cmd`.

To use the CI tools from any github pipeline:

```yaml
- name: 'Extract tykio/ci-tools:${{ matrix.tag }}'
  uses: shrink/actions-docker-extract@v3
  with:
    image: tykio/ci-tools:${{ matrix.tag }}
    path: /usr/local/bin/.
    destination: /usr/local/bin

- run: gotestsum --version
```

The action
[shrink/actions-docker-extract](https://github.com/shrink/actions-docker-extract)
is used to download and extract the CI tools binaries into your CI
workflow. The set of tools being provided can be adjusted in
[docker/tools/latest/Dockerfile](https://github.com/TykTechnologies/tyk-github-actions/blob/main/docker/tools/latest/Dockerfile).

A local Taskfile is available in `docker/tools/` that allows you to build
the tools image locally. Changes are tested in PRs.

Adoption: Internal use for PR workflows on the repository.

Source: [/.github/workflows/ci-docker-tools.yml](/.github/workflows/ci-docker-tools.yml)

## CI lint

In order to ensure some standard of quality, a lint action is being run
that checks for syntax issues, yaml issues and validates github actions
in the repository. It's not complete or fully accurate by any measure,
but it enforces conventions for the work being added in PRs.

It's generally incomplete, but extensions are welcome.

The action regenerates `README.md` from the docs/ folder contents.

To invoke the linter locally, use `task lint`.

Adoption: Internal use for PR workflows on the repository.

Source: [/.github/workflows/ci-lint.yml](/.github/workflows/ci-lint.yml)

## Create or update a GitHub comment

Undocumented action.

Source: [/.github/workflows/create-update-comment.yaml](/.github/workflows/create-update-comment.yaml)

## Print Go API Changes

For a PR, the action will print the changes in `go doc` output. This
surfaces API changes (function removals, renames, additions), as well as
comment changes.

Example usage:

```yaml
jobs:
  godoc:
    uses: TykTechnologies/github-actions/.github/workflows/godoc.yml@main
    secrets:
      ORG_GH_TOKEN: ${{ secrets.ORG_GH_TOKEN }}
```

Adoption: Gateway, Dashboard.

Source: [/.github/workflows/godoc.yml](/.github/workflows/godoc.yml)

## Golang CI

Popular linter for Go lang with good defaults.

Example usage:

```yaml
jobs:
  golangci:
    uses: TykTechnologies/github-actions/.github/workflows/golangci.yaml@main
  with:
    main_branch: master
```

Source: [/.github/workflows/golangci.yaml](/.github/workflows/golangci.yaml)

## Go test

Undocumented action.

Source: [/.github/workflows/gotest.yaml](/.github/workflows/gotest.yaml)

## Go govulncheck

Official Go Vulnerability Management.

See: https://go.dev/blog/vuln

Example usage:

```yaml
jobs:
  govulncheck:
    uses: TykTechnologies/github-actions/.github/workflows/govulncheck.yaml@main
```

Source: [/.github/workflows/govulncheck.yaml](/.github/workflows/govulncheck.yaml)

## JIRA linter

Adoption: Gateway, Dashboard.

Source: [/.github/workflows/jira-lint.yaml](/.github/workflows/jira-lint.yaml)

## Nancy Scan

OSS scanner which helps find CVEs in Go dependencies

Example usage:

```yaml
jobs:
  nancy:
    strategy:
      fail-fast: false
      matrix:
        package:
          - controller
          - dashboard
          - billing
          - monitor
          - pkg
          
    uses: TykTechnologies/github-actions/.github/workflows/nancy.yaml@main
    with:
      dir: ${{ matrix.package }}
    secrets: inherit
```

Source: [/.github/workflows/nancy.yaml](/.github/workflows/nancy.yaml)

## OWASP scanner

Example usage:

```yaml
jobs:
  owasp:
    uses: TykTechnologies/github-actions/.github/workflows/owasp.yaml@main
    with:
      target: http://staging-url.com
```

Source: [/.github/workflows/owasp.yaml](/.github/workflows/owasp.yaml)

## Release bot

```
name: Release bot

on:
  issue_comment:
    types: [created]

jobs:
  release_bot:
    uses: TykTechnologies/github-actions/.github/workflows/release-bot.yaml@main
```

## PR Agent

Undocumented action.

Source: [/.github/workflows/pr-agent.yaml](/.github/workflows/pr-agent.yaml)

## SBOM - source bill of materials (dev)

Undocumented action.

Source: [/.github/workflows/sbom-dev.yaml](/.github/workflows/sbom-dev.yaml)

## SBOM - source bill of materials

Adoption: Gateway, Dashboard.

Source: [/.github/workflows/sbom.yaml](/.github/workflows/sbom.yaml)

## Semgrep

CodeQL like OSS linter

Example usage:

```yaml
jobs:
  semgrep:
    uses: TykTechnologies/github-actions/.github/workflows/semgrep.yaml@main
```

Usage: unknown; Status: a bit out of date.

Recent images use `semgrep/semgrep`, while this workflow still uses
`returntocorp/semgrep`. Looks to be compatible at time of writing.

If you'd like to use semgrep:

- reach out to @titpetric if you need working-user assistance,
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

Source: [/.github/workflows/semgrep.yaml](/.github/workflows/semgrep.yaml)

## SonarCloud

Put it after Golang CI to automatically upload its reports to SonarCloud.

Example usage:

```yaml
jobs:
  golangci:
    uses: TykTechnologies/github-actions/.github/workflows/sonarcloud.yaml@main
  with:
    main_branch: master
    exclusions: ""
  secrets: inherit  
```

Source: [/.github/workflows/sonarcloud.yaml](/.github/workflows/sonarcloud.yaml)

## Sentinel One CNS Scans

This runs the S1 scans and publishes the results to the S1 console.
It has three available scanners.
- Secret scanner
- IaC scanner
- Vulnerability scanner

By default, all three are enabled, but it could be controlled by setting the flags appropriately
while calling the workflow.
Also, keep in mind that the secret scanner runs only on pull request events, as the scanner only supports
publishing results on pull requsts.

Example usage:

```yaml
name: SentinelOne CNS Scan

on:
  pull_request:
    types: [ opened, reopened, synchronize ]
    branches: [ master ]

jobs:
  s1_scanner:
    uses: TykTechnologies/github-actions/.github/workflows/s1-cns-scan.yml@main
    with:
      iac_enabled: false
      tag: service:vulnscan
      scope_type: ACCOUNT
    secrets:
      S1_API_TOKEN: ${{ secrets.S1_API_TOKEN }}
      CONSOLE_URL: ${{ secrets.S1_CONSOLE_URL }}
      SCOPE_ID: ${{ secrets.S1_SCOPE_ID }}
```

Source: [/.github/workflows/s1-cns-scan.yml](/.github/workflows/s1-cns-scan.yml)
