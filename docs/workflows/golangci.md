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
