## PR Checkout

The checkout PR action will fetch only the commits that belong to the PR.
This is required for various code analysis tooling, including sonarcloud.

Example usage:

```
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
