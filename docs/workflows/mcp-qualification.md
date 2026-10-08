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
