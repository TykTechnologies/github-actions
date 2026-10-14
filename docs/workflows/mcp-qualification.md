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
