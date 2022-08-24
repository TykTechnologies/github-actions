# Re-usable github actions

Collection of shared github actions which are used in our org. 

## OWASP scanner

Example usage:
```
jobs:
  owasp:
    uses: TykTechnologies/github-actions/.github/workflows/owasp.yaml@main
    with:
      target: http://staging-url.com
```

## Nancy Scan

OSS scanner which helps find CVEs in Go dependencies

Example usage:
```
jobs:
  nancy:
    strategy:
      fail-fast: false
      matrix:
        package:
          - controller
          - dashboard
          - billing
          - monitor
          - pkg
          
    uses: TykTechnologies/github-actions/.github/workflows/nancy.yaml@main
    with:
      dir: ${{ matrix.package }}
    secrets: inherit
```
