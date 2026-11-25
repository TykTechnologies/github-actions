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

## Upload Failed Job Logs

On workflow job failure, fetch and send relevant logs with branch information to an external API

Source: [/.github/actions/gh-logs-analyser/action.yaml](/.github/actions/gh-logs-analyser/action.yaml)

## Calculate tests tags

Calculates corresponding CI image tags based on github events for a group of tyk repositories

Source: [/.github/actions/latest-versions/action.yaml](/.github/actions/latest-versions/action.yaml)

## Secret scan

Scans for secrets before an image is published, and fails the job on a finding.

- **Build folder** (`path`): gitleaks with its default rules, including inside archives, plus a check for credentials in any copied `.git/config` (gitleaks skips `.git`).
- **Image** (`image`): for every platform of a multi-platform image,
  - trufflehog on every layer, the image config and the build history. This catches secrets deleted in a later layer, passed as build args, set in `ENV` or `LABEL`, or compiled into a binary;
  - gitleaks on the archives in the final filesystem (`.tar.gz`, `.tgz`, `.zip`, `.jar`, `.whl`, ...), which trufflehog does not fully unpack. Only regular archive files are copied out of the image (never links, absolute or `..` paths), so image contents cannot write outside the scan folder. An archive larger than `max-archive-mb` (default 1024) fails the scan rather than being skipped. Needs docker and python3; turn off with `scan-archives: false`.
- **Provenance attestation** of a registry image. With BuildKit's `provenance: mode=max`, every build arg value is recorded in the attestation and published with the image, including build args used only in a builder stage, which leave no trace in the final image's layers or history. On GitHub Actions it also holds the triggering event payload (commit messages included). The action fails on a non-empty build arg whose name says it is a secret (`token`, `secret`, `password`, `key`, `auth`, ...; not names ending in `_id`, `_url`, `_path`, `_file` and the like, nor `public_key`), whatever its value, and on anything gitleaks finds in the attestation. An image with no attestation passes with a warning. Turn off with `scan-attestations: false`. Pass secrets to builds with BuildKit secret mounts, never as build args.

`path` and `image` must not start with `-`. Scanner errors always fail the job, even with `enforce: false`: trufflehog exits 0 when it cannot read an image, so the action checks its log and fails if nothing was scanned. Secret values are never printed; findings show the tool, rule, file, platform and layer.

Example usage, in a release job after the image is pushed to a private registry and before it is pushed anywhere public:

```yaml
- name: Scan for secrets
  uses: TykTechnologies/github-actions/.github/actions/secret-scan@production
  with:
    path: dist
    image: ${{ steps.ecr.outputs.registry }}/my-image:${{ github.sha }}
    gitleaks-config: .gitleaks.toml                # optional
    allowlist: .github/secret-scan-allowlist.json  # optional
```

`image` takes a registry reference (pulled with the job's docker login and resolved to a digest; every platform of a multi-platform tag is scanned, and an image the job already has locally is left in place), `docker://<name>` for an image in the local daemon, or `file://<path>` for a saved tarball.

False positives:

- Build folder: pass `gitleaks-config`, a `.gitleaks.toml` that starts with `[extend]` and `useDefault = true`, then adds `[[allowlists]]` paths. Nothing inside the scanned folder or the working directory changes the rules: the action always passes its own config, ignores `gitleaks:allow` comments and does not read `.gitleaksignore` from the working directory. gitleaks always applies a `.gitleaksignore` at the root of the scanned folder, so the action reports that file as a finding (`unreviewed-gitleaksignore`).
- Archives in the image: by default all gitleaks rules except `generic-api-key`, which matches identifiers in minified JavaScript bundles. A `gitleaks-config` replaces this.
- Image layers: a JSON list of `{"detector": "...", "path": "<regex>", "reason": "..."}`; a malformed list or regex fails the scan. A default entry for dpkg checksum files in base images is built in.
- Provenance attestation: the same `allowlist`. A secret-named build arg has detector `secret-named-build-arg` and path `build-arg:<NAME>`, for example `{"detector": "secret-named-build-arg", "path": "^build-arg:GPG_KEY_ID$", "reason": "key ID, not a key"}`; a gitleaks finding has its gitleaks rule as detector and path `provenance attestation`. The gitleaks rules are the defaults, or `gitleaks-config` when given.

The `gitleaks-config` and `allowlist` files should be reviewed by AppSec (CODEOWNERS).

Reports are written to `$RUNNER_TEMP/secret-scan-results`. trufflehog's raw output there contains the secret values: never upload that folder as an artifact.

`tests/secret-scan` builds 16 images that leak fake credentials in common ways and 2 correctly built controls, plus images pushed to a local registry with and without provenance attestations; `secret-scan-test.yml` runs them, the fail-closed cases and a real multi-platform release image on every change.

Source: [/.github/actions/secret-scan/action.yml](/.github/actions/secret-scan/action.yml)

# Reusable workflows

## Branch Suggestion for PRs

Undocumented action.

Source: [/.github/workflows/branch-suggestion.yml](/.github/workflows/branch-suggestion.yml)

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
  uses: shrink/actions-docker-extract@v4
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

## CI Tests

Undocumented action.

Source: [/.github/workflows/ci-test.yml](/.github/workflows/ci-test.yml)

## Create or update a GitHub comment

Undocumented action.

Source: [/.github/workflows/create-update-comment.yaml](/.github/workflows/create-update-comment.yaml)

## Dependency Change Guard

Undocumented action.

Source: [/.github/workflows/dependency-guard.yml](/.github/workflows/dependency-guard.yml)

## Docs preview

Renders the generated-documentation diff (config docs, OAS/x-tyk-gateway,
swagger) for a PR in a source repo, posts it as a sticky PR comment, and
requests an **advisory** review from a PM team. The goal is to catch PM
feedback on doc-bearing code changes before they merge, instead of after the
[tyk-docs sync](https://github.com/TykTechnologies/exp/actions/workflows/tyk-docs.yml)
has already opened a docs PR (which forces a fix-regenerate-review loop).

Behavior:

- No touched doc-bearing path → the workflow ends silently.
- Touched paths but identical generated output (pure refactor) → updates an
  existing preview comment to "No documentation impact"; stays silent otherwise.
- Generated output changed → sticky comment with the rendered docs diff
  (truncated at 60k chars; full diff as the `docs-preview-diff` artifact) and a
  one-time advisory review request to `pm_team`.
- Gateway or Dashboard swagger output changed → after the comment, the head
  swagger runs the same validation and codegen checks as the sync, so a swagger
  the sync would reject turns the run red before merge.
- Fork PRs are skipped (secrets unavailable to forks).
- Never make this a required check — it is advisory by design.

Inputs:

- `component` (required): `gateway`, `dashboard`, `pump`, `mdcb` or `portal` —
  selects the generator set, mirroring the tyk-docs sync jobs.
- `docs_paths` (required): newline-separated globs of doc-bearing paths,
  matched against the PR's changed files.
- `pm_team` (optional): GitHub team slug to request as advisory reviewer.
- `comment_marker` (optional): sticky-comment identity marker.

Secrets: `PROBE_APP_ID`, `PROBE_APP_PRIVATE_KEY` (org-wide App; used for the
private `tyk-config-info-generator` checkout and the team review request).

Example usage (tyk):

```yaml
name: Docs preview

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  docs-preview:
    uses: TykTechnologies/github-actions/.github/workflows/docs-preview.yml@main
    with:
      component: gateway
      docs_paths: |
        apidef/oas/**
        config/**
        swagger.yml
      pm_team: pms
    secrets: inherit
```

Generation logic intentionally mirrors exp's `tyk-docs.yml`; if you change
one, change the other (until both consume shared composite actions).

Source: [/.github/workflows/docs-preview.yml](/.github/workflows/docs-preview.yml)

## Gromit Drift Check

Undocumented action.

Source: [/.github/workflows/drift-check.yml](/.github/workflows/drift-check.yml)

## Force Merge PR (Reusable)

Undocumented action.

Source: [/.github/workflows/force-merge.yaml](/.github/workflows/force-merge.yaml)

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

Validates pull requests against Jira tickets. Extracts Jira issue IDs from branch names or PR titles, fetches issue 
details from Jira, updates PR descriptions with ticket information, and validates issue status.

### Features

- Extracts Jira issue IDs from branch names (e.g., `feature/ABC-123-description`)
- Validates PR title contains matching Jira ticket ID
- Fetches Jira issue details via API
- Updates PR description with ticket details (status, summary, link)
- Validates issue status against accepted statuses
- Posts sticky PR comment on failure with error details

### Usage as a reusable workflow

```yaml
jobs:
  jira-lint:
    uses: TykTechnologies/github-actions/.github/workflows/jira-lint.yaml@main
    secrets:
      JIRA_USER_EMAIL: ${{ secrets.JIRA_USER_EMAIL }}
      JIRA_TOKEN: ${{ secrets.JIRA_TOKEN }}
```

### Usage as a composite action

```yaml
jobs:
  jira-lint:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
      contents: read
    steps:
      - uses: TykTechnologies/github-actions/jira-linter@main
        with:
          jira-base-url: 'https://tyktech.atlassian.net'
          jira-user-email: ${{ secrets.JIRA_USER_EMAIL }}
          jira-api-token: ${{ secrets.JIRA_TOKEN }}
```

### Secrets

| Secret            | Description                              |
|-------------------|------------------------------------------|
| `JIRA_USER_EMAIL` | Email associated with the Jira API token |
| `JIRA_TOKEN`      | Jira API token for authentication        |

Adoption: Gateway, Dashboard.

Source: [/.github/workflows/jira-lint.yaml](/.github/workflows/jira-lint.yaml)

## Serial MCP stack qualification

Builds Gateway, Dashboard, Pump and the mock MCP server from source, then runs
Dashboard-owned MCP tests serially across MessagePack/protobuf and
MongoDB/PostgreSQL. Gateway, Dashboard and Pump each call this public workflow
so that a producer change is tested at its own pull request head.

The caller component uses `github.event.pull_request.head.sha`, rather than the
synthetic merge revision. Other components use the same source branch when it
exists, falling back to the caller's base branch. Resolved commit SHAs, binary
versions and sanitized results are retained as artifacts. Mock fixtures are
pinned to an immutable commit. The existing declared skips remain visible.

Use an immutable commit of this repository containing the workflow:

```yaml
jobs:
  mcp-qualification:
    name: Coordinated MCP qualification
    needs: dep-guard
    if: github.event_name == 'pull_request' && github.event.pull_request.draft == false
    uses: TykTechnologies/github-actions/.github/workflows/mcp-qualification.yml@<full-commit-sha>
    secrets:
      PROBE_APP_ID: ${{ secrets.PROBE_APP_ID }}
      PROBE_APP_PRIVATE_KEY: ${{ secrets.PROBE_APP_PRIVATE_KEY }}
      DASH_LICENSE: ${{ secrets.DASH_LICENSE }}
    permissions:
      contents: read
```

Keep `mcp-qualification` in the caller's aggregate job dependencies. Excluding
MCP from parallel API tests requires this mandatory serial replacement. The
Dashboard test runner, requirements and compose fixtures stay in
`tyk-analytics/tests/api`; do not copy them into this repository.

Prerequisites:

- Linux runner with Docker Compose, GitHub CLI and sufficient space for four
  independent Go builds; the caller's `WARP_RUNNER_8X_X64` or `DEFAULT_RUNNER`
  variable selects it.
- GitHub App credentials with read access to `tyk`, private `tyk-analytics`,
  `tyk-pump`, `tyk-mock-mcp-server`, private `tyk-sync-internal` and private
  `api-definition`.
- Dashboard license in `DASH_LICENSE`.
- A selected Dashboard revision containing `run_mcp_v2_stack.py`, its helper
  regressions and `fixtures/mcp_v2/compose.yml`.

Only the three named secrets are forwarded. Checkouts do not persist
credentials. The workflow creates a read-only App token, uses isolated local
backing services and stops its own compose project on completion. Evidence
uploads exclude service logs and credentials.

Roll out the shared workflow before pinning callers to its SHA. Update gromit
release templates and enable their `mcp-qualification` feature only on branches
whose selected Dashboard revision contains the required runner. Then regenerate
the consumer release workflows and remove their duplicated local qualification
workflow. Do not merge consumer references to a commit that is not yet published.

Source: [/.github/workflows/mcp-qualification.yml](/.github/workflows/mcp-qualification.yml)

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

## Path-Based OSV Scan

Undocumented action.

Source: [/.github/workflows/osv-path-scan.yml](/.github/workflows/osv-path-scan.yml)

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

Source: [/.github/workflows/release-bot.yaml](/.github/workflows/release-bot.yaml)

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

## SBOM - source bill of materials (dev)

Undocumented action.

Source: [/.github/workflows/sbom-dev.yaml](/.github/workflows/sbom-dev.yaml)

## SBOM - source bill of materials

Adoption: Gateway, Dashboard.

Source: [/.github/workflows/sbom.yaml](/.github/workflows/sbom.yaml)

## Secret scan tests

Tests for the secret-scan action. Builds images that leak fake credentials in common ways, plus correctly built controls, and checks every leak is caught, the controls pass, a real multi-platform release image is clean on every platform, and the action fails closed when a scanner cannot run.

Adoption: Internal use for PR workflows on the repository.

Source: [/.github/workflows/secret-scan-test.yml](/.github/workflows/secret-scan-test.yml)

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

## Upgrade Tests

Undocumented action.

Source: [/.github/workflows/upgrade-tests.yml](/.github/workflows/upgrade-tests.yml)

## Visor

Undocumented action.

Source: [/.github/workflows/visor.yaml](/.github/workflows/visor.yaml)

