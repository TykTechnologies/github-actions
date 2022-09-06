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

## Semgrep
CodeQL like OSS linter

Example usage:
```
jobs:
  semgrep:
    uses: TykTechnologies/github-actions/.github/workflows/semgrep.yaml@main
```


## Golang CI
Popular linter for Go lang with good defaults

Example usage:
```
jobs:
  golangci:
    uses: TykTechnologies/github-actions/.github/workflows/golangci.yaml@main
  with:
    main_branch: master
```

## SonarCloud

Put it after Golang CI to automatically upload its reports to SonarCloud

Example usage:
```
jobs:
  golangci:
    uses: TykTechnologies/github-actions/.github/workflows/sonarcloud.yaml@main
  with:
    main_branch: master
    exclusions: ""
  secrets: inherit  
```

## Go govulncheck
Official Go Vulnerability Management
See https://go.dev/blog/vuln

Example usage:
```
jobs:
  govulncheck:
    uses: TykTechnologies/github-actions/.github/workflows/govulncheck.yaml@main
```
