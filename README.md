# Re-usable github actions

Collection of shared github actions which are used in our org. 

## OWASP scanner

```
jobs:
  owasp:
    uses: TykTechnologies/github-actions/.github/workflows/owasp.yaml@main
    with:
      target: http://staging-url.com
```
