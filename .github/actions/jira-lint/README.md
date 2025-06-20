# Reliable Jira Issue Validation Action

A Go-based GitHub Action that provides reliable validation of Jira issue IDs in pull requests with improved state checking and comment management. This implementation leverages existing tools from the Tyk ecosystem for maximum reliability and consistency.

## Key Features

### 🔧 **Integration with Existing Tools**
- **Uses existing jira-lint**: Leverages `/Users/laurentiughiur/go/src/github.com/TykTechnologies/exp/cmd/jira-cli` for Jira validation
- **Reuses create-update-comment workflow**: Uses `.github/workflows/create-update-comment.yaml` for consistent comment management
- **No duplicate dependencies**: Builds on proven tools already in your ecosystem

### 🔍 **Comprehensive Validation**
- **PR Title Validation** (mandatory): Ensures PR titles contain valid Jira issue IDs
- **Branch Name Validation**: Checks for Jira issue IDs in branch names
- **PR Body Validation**: Optionally validates Jira issue references in PR descriptions
- **Consistency Checking**: Ensures matching Jira IDs across title, branch, and body

### 🎯 **Supported Jira Issue Patterns**
- `[A-Z]{2}-[0-9]{4,5}` - Standard format (e.g., TT-0000 through TT-99999)
- `SYSE-[0-9]+` - Specific project patterns (e.g., SYSE-339)
- `[A-Z]+-[0-9]+` - General project patterns

### 🚀 **Reliability Improvements**
- **No Duplicate Comments**: Uses existing create-update-comment workflow
- **Reliable State Validation**: Uses the proven jira-cli tool for consistent validation
- **Fresh Validation**: No caching issues - validates state on each run
- **Consistent Interface**: Maintains same interface as previous 3rd party actions

## Architecture

### Two-Job Workflow Design

The implementation uses a two-job workflow pattern to properly integrate with existing tools:

1. **jira-validation job**: Runs the Go action to validate Jira issues
2. **create-comment job**: Uses the existing `create-update-comment.yaml` workflow

This separation allows us to:
- Use the existing comment management workflow
- Maintain clean separation of concerns
- Follow established patterns in your codebase

### Integration with jira-lint

The action uses the existing `jira-lint` tool for Jira validation:

1. **Installation**: Automatically installs jira-lint via `go install github.com/TykTechnologies/exp/cmd/jira-lint@main`
2. **Environment Setup**: Configures required environment variables:
   - `JIRA_API_TOKEN`
   - `JIRA_API_EMAIL` 
   - `JIRA_API_URL`
3. **Validation**: Executes jira-lint for each detected issue
4. **Status Checking**: Uses jira-lint's built-in status validation against allowed states:
   - `In Dev`
   - `In Code Review`
   - `Ready for Testing`
   - `In Test`
   - `In Progress`
   - `In Review`

### Comment Management

Uses the existing `create-update-comment.yaml` workflow:

1. **Validation Output**: The action outputs a validation report
2. **Workflow Integration**: The main workflow passes the report to create-update-comment
3. **No Duplicates**: Leverages the existing logic to update rather than create new comments

## Usage

### Workflow Integration

```yaml
name: JIRA linter

on:
  workflow_call:
    secrets:
      JIRA_TOKEN:
        required: true
      JIRA_EMAIL:
        required: true
      ORG_GH_TOKEN:
        required: true

jobs:
  jira-validation:
    runs-on: ubuntu-latest
    outputs:
      validation-report: ${{ steps.validate.outputs.report }}
      validation-status: ${{ steps.validate.outputs.status }}
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4
        
      - name: Run Jira Validation
        id: validate
        uses: ./.github/actions/jira-lint
        with:
          github-token: ${{ secrets.ORG_GH_TOKEN }}
          jira-token: ${{ secrets.JIRA_TOKEN }}
          jira-email: ${{ secrets.JIRA_EMAIL }}
          jira-base-url: https://tyktech.atlassian.net
          skip-branches: '^(release-[0-9.-]+(lts)?|master|main)$'
          validate_issue_status: true
          skip-comments: true

  create-comment:
    needs: jira-validation
    if: ${{ needs.jira-validation.outputs.validation-report != '' }}
    uses: ./.github/workflows/create-update-comment.yaml
    with:
      body-includes: "<!-- jira-lint-comment -->"
      body: ${{ needs.jira-validation.outputs.validation-report }}
```

## Input Parameters

| Parameter | Description | Required | Default |
|-----------|-------------|----------|---------|
| `github-token` | GitHub token for API access | ✅ | - |
| `jira-token` | Jira API token | ✅ | - |
| `jira-email` | Jira API email | ✅ | - |
| `jira-base-url` | Jira base URL (e.g., https://tyktech.atlassian.net) | ✅ | - |
| `skip-branches` | Regex pattern for branches to skip validation | ❌ | `''` |
| `skip-comments` | Skip adding lint comments for PR title | ❌ | `false` |
| `validate_issue_status` | Validate Jira issue status using jira-lint | ❌ | `false` |

## Validation Logic

### 1. Title Validation (Required)
- ✅ **Pass**: PR title contains valid Jira issue ID
- ❌ **Fail**: No Jira issue ID found in title

### 2. Branch Validation (Optional)
- ✅ **Pass**: Branch name contains Jira issue ID
- ⚠️ **Warning**: No Jira issue ID in branch name

### 3. Consistency Check
- ✅ **Pass**: Same Jira issue ID across title and branch
- ❌ **Fail**: Different Jira issue IDs in title vs branch

### 4. Status Validation (Optional)
When `validate_issue_status: true`:
- ✅ **Pass**: jira-lint validation succeeds (issue exists and status is allowed)
- ❌ **Fail**: jira-lint validation fails (issue doesn't exist or status not allowed)

## Example Validation Report

```markdown
## 🔍 Jira Issue Validation Report

### PR Title