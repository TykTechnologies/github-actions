## Keepalive PR

Creates a temporary pull request and closes it after a configurable delay.
This is intended for low-traffic repositories where scheduled workflows may be
disabled after long periods without repository activity.

Example usage:

```yaml
jobs:
  open:
    uses: TykTechnologies/github-actions/.github/workflows/keepalive-pr.yaml@main
    permissions:
      contents: write
      pull-requests: write
    with:
      mode: open
      base-branch: main

  close:
    uses: TykTechnologies/github-actions/.github/workflows/keepalive-pr.yaml@main
    permissions:
      contents: write
      issues: write
      pull-requests: write
    with:
      mode: close
      close-after-minutes: "10"
```

Grant at least the permissions shown on each calling job. The reusable workflow
scopes its internal jobs the same way.

Set `base-branch` explicitly (for example `main`) when the caller does not run
on the default branch ref. In a `workflow_call`, `GITHUB_REF_NAME` reflects the
caller context and may be wrong for tags, pull requests, or other refs.

Open and close runs are serialized per repository via workflow concurrency, so
only one keepalive operation mutates branches and pull requests at a time.

`mode` must be exactly `open` or `close`. Any other value fails the workflow
during input validation instead of skipping both jobs silently.
