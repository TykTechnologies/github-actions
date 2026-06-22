# Jira Linter

A GitHub Action tool that validates pull requests against Jira tickets. It extracts Jira issue IDs from branch names, 
fetches issue details, validates ticket status, and updates PR descriptions with ticket information.

## Features

- Extracts Jira issue IDs from branch names (e.g., `ABC-123`)
- Validates that PR title contains matching Jira ticket ID
- Fetches Jira issue details (title, status) via Jira API
- Updates PR description with ticket information
- Validates issue status against accepted statuses
- Provides clear error messages for validation failures

## Usage

### As a GitHub Action

Add this action to your workflow file (e.g., `.github/workflows/pr-validation.yml`):

```yaml
name: Validate PR against Jira

on:
  pull_request:
    types: [opened, synchronize, reopened, edited]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Validate Jira ticket
        uses: TykTechnologies/github-actions/jira-linter@main
        with:
          jira-base-url: 'https://your-org.atlassian.net'
          jira-user-email: 'your-email@example.com'
          jira-api-token: ${{ secrets.JIRA_API_TOKEN }}
```

**Inputs:**

- `jira-base-url` (required): Base URL of your Jira instance
- `jira-user-email` (required): Email associated with Jira API token
- `jira-api-token` (required): Jira API token (store in GitHub secrets)

**Behavior:**

- Automatically extracts branch name from PR context
- Validates PR title and Jira ticket status
- Updates PR description with ticket details
- Posts comment on failure with error details
- Auto-deletes failure comment when validation passes

### As a Command Line Tool

**Prerequisites:**

Set up environment variables:

```bash
export JL_JIRA_BASEURL="https://your-domain.atlassian.net"
export JL_JIRA_USEREMAIL="your-email@example.com"
export JL_JIRA_APITOKEN="your-api-token"
export JL_PR_NUMBER=123
export JL_PR_TITLE="ABC-123: Your PR title"
export GITHUB_TOKEN="your-github-token"
export GITHUB_REPOSITORY="owner/repo"
```

**Command:**

```bash
linter -branch "feature/ABC-123-your-feature" [-statuses "In Dev,In Code Review"]
```

**Parameters:**

- `-branch` (required): Branch name containing Jira ticket ID
- `-statuses` (optional): Comma-separated list of accepted Jira statuses. Defaults to: "In Dev", "In Code Review", "Ready For Dev"

**Examples:**

```bash
# Basic usage with default statuses
linter -branch "feature/ABC-123-implement-auth"

# Custom accepted statuses
linter -branch "hotfix/XYZ-456-fix-bug" -statuses "In Progress,Testing,Done"

# Skip status validation
linter -branch "chore/DEF-789-update-deps" -statuses ""
```

## Authentication

### Jira API Token

1. Generate API token from [Atlassian Account Settings](https://id.atlassian.com/manage-profile/security/api-tokens)
2. Set `JL_JIRA_USEREMAIL` to the email associated with the token
3. Set `JL_JIRA_APITOKEN` to the raw API token (no base64 encoding needed)
4. For GitHub Action, store both values in GitHub Secrets

### GitHub Token

The `GITHUB_TOKEN` is automatically provided by GitHub Actions with necessary permissions.

## Validation Rules

1. **Branch name** must contain a valid Jira ticket ID (format: `LETTERS-NUMBERS`)
2. **PR title** must contain the same Jira ticket ID as the branch
3. **Jira issue** must exist and be accessible
4. **Issue status** must match one of the accepted statuses (unless validation is skipped)

## Error Messages

- `branch name must contain a valid Jira ticket ID`: Branch doesn't follow naming convention
- `PR title must contain the Jira ticket ID`: PR title missing ticket ID
- `PR title contains 'X' but branch has 'Y'`: Mismatched ticket IDs
- `failed to fetch Jira issue`: Cannot access Jira (check credentials/network)
- `has status 'X' but must be one of: Y`: Issue status not acceptable

## Exit Codes

- `0`: Validation successful
- `1`: Validation failed or error occurred