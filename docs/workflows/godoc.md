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
