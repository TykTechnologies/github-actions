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
      pm_team: product-managers
    secrets: inherit
```

Generation logic intentionally mirrors exp's `tyk-docs.yml`; if you change
one, change the other (until both consume shared composite actions).
