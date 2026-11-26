## Dependency Change Guard

Fails a pull request that changes dependency files (`go.mod`, `go.sum`,
`vendor/`, npm and Python lock and requirement files, plus `extra_paths`)
until a reviewer has checked the change and added the `deps-reviewed` label
(`label_name`). On pushes the check is skipped and the job succeeds.

The job reads the PR's **current** labels, so after adding the label a
reviewer re-runs the failed jobs; no new push and no `labeled` trigger is
needed. This needs `pull-requests: read`. A called workflow can only lower
the caller's permissions, so set them where you call it:

```yaml
dep-guard:
  if: github.event_name == 'pull_request'
  uses: TykTechnologies/github-actions/.github/workflows/dependency-guard.yml@production
  permissions:
    contents: read
    pull-requests: read
```

Without `pull-requests: read` the job uses the labels from the event that
started the run, as it did before, and logs a notice. A re-run then does not
see a label added later; only a new run (a push, or a `labeled` trigger) does.

Jobs that depend on it should accept `success` or `skipped`:

```yaml
needs: [dep-guard]
if: ${{ !cancelled() && (needs.dep-guard.result == 'success' || needs.dep-guard.result == 'skipped') }}
```

Inputs:

- `label_name`: the label that skips the check (default `deps-reviewed`).
- `extra_paths`: more paths to watch, space-separated.

Adoption: Release in gromit-managed repos (tyk, tyk-analytics, portal, tyk-pump,
tyk-identity-broker, tyk-sink), tyk-analytics-ui and other repos.
