## Serial MCP stack qualification

Builds Gateway, Dashboard, Pump and the mock MCP server from source, then runs
Dashboard-owned MCP tests serially across MessagePack/protobuf and
MongoDB/PostgreSQL. Gateway, Dashboard, Pump and MDCB callers use this public
workflow to test each component at its own pull request head.

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

Checkouts do not persist credentials. The workflow creates a read-only App
token, uses isolated local backing services and stops its own compose project
on completion. Evidence uploads exclude service logs and credentials.

Caller workflow pins are defined in the Gromit release templates. Enable
`mcp-qualification` only on branches whose selected Dashboard revision contains
the required runner and fixtures.

For MDCB qualification, set `with: {topology: mdcb}` and forward
`MDCB_LICENSE: ${{ secrets.MDCB_LICENSE }}` alongside `DASH_LICENSE`.
The topology defaults to `direct`; MDCB requires its licence and additionally
resolves and builds `tyk-sink`. Give the App read access to that repository.
A sink caller uses its exact PR head. Dashboard and the other components use
the coordinated branch with the same existing base-branch fallback.

The four MDCB jobs cover MongoDB/PostgreSQL × MessagePack/Protobuf. Each job
starts a primary Gateway, MDCB, and a data-plane Gateway with task-owned Redis.
The runner boots one hybrid organisation and RPC identity, proves API, key and
policy propagation, and sends test traffic to the data plane. MDCB persists
analytics directly (`forward_analytics_to_pump=false`); forwarded analytics
is outside this matrix. Missing prerequisites fail qualification.

Artifacts include sanitized JUnit status, source commits, binary hashes and
routing/persistence evidence. Protected logs, runtime identity and licences
are excluded. The runner deletes only its organisation and data-plane Redis;
the workflow removes its dedicated Compose project and module authentication.

### Integrated API-test execution

The `mcp-api-tests` policy uses the standard test controller and `api-tests` job
for MCP qualification. The controller expands existing API compatibility rows
and adds 16 MCP rows. It resolves component branch heads once, waits for PR-head
images, and freezes image digests and the `tyk-pro` environment revision before
fan-out. Exact caller heads, immutable mock fixtures and source labels are
verified. Missing peer images fail rather than selecting mutable tags.

The existing environment checkout, test checkout, `env-up`, API test and
reporting actions accept optional MCP inputs. Other users retain their defaults.
MCP rows use `tyk-pro/mcp.yml` with the existing datastore/cache overlays,
Dashboard-owned runtime fixtures and serial module partitioning. No applications
are compiled in test jobs. Each row uses its own Linux runner and owned stores,
mock and RPC identity. Host networking preserves pytest's localhost fixtures.

Reporting retains sanitized inventory/JUnit with shard-specific artifact and S3
names. It removes the owned RPC identity, redacts configuration and stops the
Compose project even on setup or test failure. Gateway log assertions use a
protected Docker log stream; raw MCP logs are excluded from uploads.

The required `aggregator-ci-test` consumes MCP evidence before its existing
status aggregation. It rejects missing or overlapping coverage, incomplete
results, unexpected skips, failed qualification checks, inconsistent source
revisions, image digests or environment revision. This path generates no
separate MCP qualification job. Publish these optional action interfaces before
regenerating callers at one immutable shared-action commit.
