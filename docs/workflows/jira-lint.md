## JIRA linter

Validates pull requests against Jira tickets. Extracts Jira issue IDs from branch names or PR titles, fetches issue 
details from Jira, updates PR descriptions with ticket information, and validates issue status.

### Features

- Extracts Jira issue IDs from branch names (e.g., `feature/ABC-123-description`)
- Validates PR title contains matching Jira ticket ID
- Fetches Jira issue details via API
- Updates PR description with ticket details (status, summary, link)
- Validates issue status against accepted statuses
- Posts sticky PR comment on failure with error details

### Usage as a reusable workflow

```yaml
jobs:
  jira-lint:
    uses: TykTechnologies/github-actions/.github/workflows/jira-lint.yaml@main
    secrets:
      JIRA_USER_EMAIL: ${{ secrets.JIRA_USER_EMAIL }}
      JIRA_TOKEN: ${{ secrets.JIRA_TOKEN }}
```

### Usage as a composite action

```yaml
jobs:
  jira-lint:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
      contents: read
    steps:
      - uses: TykTechnologies/github-actions/jira-linter@main
        with:
          jira-base-url: 'https://tyktech.atlassian.net'
          jira-user-email: ${{ secrets.JIRA_USER_EMAIL }}
          jira-api-token: ${{ secrets.JIRA_TOKEN }}
```

### Secrets

| Secret            | Description                              |
|-------------------|------------------------------------------|
| `JIRA_USER_EMAIL` | Email associated with the Jira API token |
| `JIRA_TOKEN`      | Jira API token for authentication        |

Adoption: Gateway, Dashboard.
