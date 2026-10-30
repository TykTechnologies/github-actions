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
