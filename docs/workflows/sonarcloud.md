## SonarCloud

Put it after Golang CI to automatically upload its reports to SonarCloud.

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
