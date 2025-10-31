# Branch Suggestion Automation

Automated branch suggestion tool that analyzes JIRA fix versions and suggests appropriate merge target branches for pull requests based on the repository's branching strategy.

## Features

- Extracts JIRA ticket from PR title or branch name
- Fetches fix versions from JIRA
- Matches fix versions to repository branches using deterministic rules
- Posts/updates a comment on PRs with suggested merge targets
- Supports both local testing (Visor) and GitHub Actions integration

## Quick Start

### For Repository Maintainers

Add this workflow to your repository to enable automatic branch suggestions:

1. Copy `.github/workflows/example-usage.yml.template` to your repository as `.github/workflows/branch-suggestion.yml`

2. Add required secrets to your repository (Settings → Secrets and variables → Actions):
   - `JIRA_API_TOKEN`: JIRA API token (generate at https://id.atlassian.com/manage-profile/security/api-tokens)

3. That's it! The workflow will automatically analyze PRs and post branch suggestions.

### Example Workflow Configuration

```yaml
# .github/workflows/branch-suggestion.yml
name: PR Branch Suggestions

on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions:
  pull-requests: write
  contents: read

jobs:
  branch-suggestions:
    uses: TykTechnologies/REFINE/.github/workflows/branch-suggestion.yml@main
    secrets:
      JIRA_EMAIL: ${{ secrets.JIRA_EMAIL }}
      JIRA_API_TOKEN: ${{ secrets.JIRA_API_TOKEN }}
```

## How It Works

### 1. JIRA Ticket Extraction

The tool looks for JIRA ticket keys in the PR title or branch name:
- Pattern: `TT-12345`, `TYK-456`, etc.
- Examples:
  - "TT-12345: Fix authentication bug" → `TT-12345`
  - "feature/TT-12345-fix-auth" → `TT-12345`
  - "Fix auth (TT-12345)" → `TT-12345`

### 2. Fix Version Fetching

Retrieves the "Fix Version/s" field from the JIRA ticket, which determines which product versions the fix should be merged into.

### 3. Branch Matching

Uses deterministic rules to match fix versions to branches:

#### Priority Levels
- **Required** (✅): Exact version match or release branch
- **Recommended** (📌): Minor version match or LTS branch
- **Optional** (💡): Major version match or main branch

#### Matching Rules
1. **Exact Match**: `5.8.1` → `release-5.8.1` (Required)
2. **Minor Version**: `5.8.1` → `release-5.8` (Required)
3. **Major Version**: `5.8.1` → `release-5` (Recommended)
4. **LTS Branch**: `5.3.x LTS` → `release-5.3-lts` (Required)
5. **Main Branch**: Always suggested as Optional

### 4. PR Comment

Posts a comment on the PR with:
- JIRA ticket information
- List of suggested branches organized by fix version
- Priority indicators (required/recommended/optional)
- Merging instructions

## Local Testing

### Prerequisites

1. Install dependencies:
```bash
npm install
```

2. Install Visor (if not already installed):
```bash
npm install -g @probelabs/visor
```

3. Set up environment variables:
```bash
export JIRA_EMAIL="your-email@example.com"
export JIRA_API_TOKEN="your-jira-api-token"
```

### Test Individual Scripts

#### Test JIRA Fix Version Fetcher

```bash
# Direct ticket key
node scripts/jira/get-fixedversion.js TT-12345

# From PR title
node scripts/jira/get-fixedversion.js "TT-12345: Fix authentication bug"

# From branch name
node scripts/jira/get-fixedversion.js "feature/TT-12345-fix-auth"
```

**Expected output:**
```json
{
  "ticket": "TT-12345",
  "summary": "Fix authentication bug",
  "priority": "High",
  "issueType": "Bug",
  "fixVersions": [
    {
      "name": "5.8.1",
      "id": "12345",
      "released": false,
      "parsed": {
        "major": 5,
        "minor": 8,
        "patch": 1,
        "original": "5.8.1"
      }
    }
  ]
}
```

**Exit codes:**
- `0`: Success (fix versions found)
- `1`: Error (ticket found but no fix versions set)
- `2`: No JIRA ticket found

#### Test Branch Matcher

```bash
# Create test data
echo '{
  "ticket": "TT-12345",
  "fixVersions": [
    {"name": "5.8.1", "parsed": {"major": 5, "minor": 8, "patch": 1}}
  ]
}' > /tmp/jira.json

echo '[
  {"name": "main"},
  {"name": "release-5.8"},
  {"name": "release-5.8.1"}
]' > /tmp/branches.json

# Run matcher
node scripts/common/match-branches.js \
  "$(cat /tmp/jira.json)" \
  "$(cat /tmp/branches.json)"
```

#### Test PR Comment Posting

```bash
# Create test comment
echo "## Test Comment\nThis is a test." > /tmp/comment.md

# Post to PR (requires GITHUB_TOKEN)
export GITHUB_TOKEN="your-github-token"

node scripts/github/add-pr-comment.js \
  TykTechnologies/tyk \
  123 \
  --file /tmp/comment.md
```

### Test Complete Pipeline with Visor

```bash
# Test with a real ticket
env JIRA_EMAIL="your-email@example.com" \
    JIRA_API_TOKEN="your-token" \
    PR_TITLE="TT-12345" \
    REPOSITORY="TykTechnologies/tyk" \
    visor --config branch_suggestion.yml

# Test with TIB ticket (different version format)
env JIRA_EMAIL="your-email@example.com" \
    JIRA_API_TOKEN="your-token" \
    PR_TITLE="TT-5433" \
    REPOSITORY="TykTechnologies/tyk-identity-broker" \
    visor --config branch_suggestion.yml
```

**Expected output:**
```
========================================
BRANCH SUGGESTION ANALYSIS
========================================
📝 PR Title: TT-12345
🌿 Branch: feature/TT-12345-test
📦 Repository: TykTechnologies/tyk
========================================

🔍 STEP 1: Fetching JIRA ticket and fix versions...
✅ JIRA Ticket: TT-12345
   Summary: Fix authentication bug
   Fix Versions: 1
     - 5.8.1 (released: false)

🔍 STEP 2: Fetching repository branches...
✅ Branches fetched: 45
     - main
     - release-5.8
     - release-5.8.1
     ...

🔍 STEP 3: Matching branches using deterministic rules...
✅ Branch matching complete

========================================
✅ ANALYSIS COMPLETE
========================================

📋 JIRA Ticket: TT-12345
   Summary: Fix authentication bug

🎯 Fix Version: 5.8.1
   ✅ release-5.8.1 - Exact version match
   ✅ release-5.8 - Minor version match
   📌 release-5 - Major version match
   💡 main - Main development branch
```

## Project Structure

```
.
├── .github/workflows/
│   ├── branch-suggestion.yml           # Reusable GitHub Actions workflow
│   └── example-usage.yml.template      # Template for other repositories
├── branch_suggestion.yml               # Visor pipeline configuration
├── scripts/
│   ├── jira/
│   │   ├── get-fixedversion.js        # Extract ticket and fetch fix versions
│   │   └── jira-api.js                # JIRA API wrapper
│   ├── github/
│   │   ├── add-pr-comment.js          # Create/update PR comments
│   │   └── github-api.js              # GitHub API wrapper
│   └── common/
│       └── match-branches.js          # Branch matching logic
└── schemas/
    └── branch-suggestion.json         # JSON schema for output
```

## Configuration

### Environment Variables

#### Required
- `JIRA_EMAIL`: JIRA account email
- `JIRA_API_TOKEN`: JIRA API token

#### Optional (for PR comment posting)
- `GITHUB_TOKEN`: GitHub token (automatically provided in GitHub Actions)
- `PR_NUMBER`: Pull request number
- `REPOSITORY`: Repository in format `owner/repo`

### Visor Configuration

The `branch_suggestion.yml` file contains the Visor pipeline configuration with two main steps:

1. **analyze-and-suggest**: Combines JIRA fetching, branch matching, and output generation
2. **post-pr-comment**: Posts or updates a PR comment (only runs with `--tags remote`)

### Branching Strategy Support

The tool automatically adapts to different branching strategies:

#### Release Branches
- `release-5.8.1` (patch releases)
- `release-5.8` (minor releases)
- `release-5` (major releases)

#### LTS Branches
- `release-5.3-lts`
- `lts-5.3`
- `5.3-lts`

#### Special Branches
- `main` / `master` (main development)
- Feature branches (not suggested)
- Hotfix branches (not suggested)

## Troubleshooting

### No JIRA ticket found

**Symptom:** Exit code 2, message "No JIRA ticket found in input"

**Solution:** Ensure PR title or branch name contains a JIRA ticket key in format `TT-12345`

### No fix versions set

**Symptom:** Exit code 1, message "No fix versions found in JIRA ticket"

**Solution:** Set the "Fix Version/s" field in JIRA before creating the PR

### JIRA API authentication failed

**Symptom:** Error message about authentication

**Solution:**
1. Verify `JIRA_EMAIL` matches your JIRA account email
2. Generate a new API token at https://id.atlassian.com/manage-profile/security/api-tokens
3. Ensure the token has not expired

### GitHub API rate limiting

**Symptom:** Error 403 from GitHub API

**Solution:**
1. Use a GitHub token with sufficient rate limits
2. In GitHub Actions, the automatic `GITHUB_TOKEN` has higher rate limits
3. For local testing, create a personal access token with `repo` scope

### Control characters in output

**Symptom:** JSON parse error about control characters

**Solution:** This has been fixed in the latest version by using regex-based extraction instead of JSON parsing. Update to the latest version of the tool.

### dotenv logging pollution

**Symptom:** `[dotenv@17.2.1]` messages in output

**Solution:** Already fixed by setting `DOTENV_LOG_LEVEL=error` before loading dotenv. Update to the latest version.

## Output Schema

The tool outputs JSON conforming to the schema in `schemas/branch-suggestion.json`:

```json
{
  "ticket": "TT-12345",
  "summary": "Fix authentication bug",
  "priority": "High",
  "issueType": "Bug",
  "fixVersions": [...],
  "matchResults": [
    {
      "fixVersion": "5.8.1",
      "branches": [
        {
          "branch": "release-5.8.1",
          "reason": "Exact version match",
          "priority": "required"
        }
      ]
    }
  ],
  "markdown": "# Branch Suggestions..."
}
```

## Contributing

### Adding New Branching Patterns

Edit `scripts/common/match-branches.js` and add new patterns to the `matchBranches` function.

### Modifying Priority Rules

Update the priority assignment logic in `scripts/common/match-branches.js`.

### Customizing PR Comment Format

Modify the markdown template generation in `scripts/common/match-branches.js`.

## API Documentation

### `get-fixedversion.js`

```javascript
import { extractJiraTicket, getFixVersions } from './scripts/jira/get-fixedversion.js';

// Extract ticket from text
const ticket = extractJiraTicket('TT-12345: Fix bug');
// Returns: 'TT-12345'

// Get fix versions
const result = await getFixVersions('TT-12345');
// Returns: { ticket, summary, priority, issueType, fixVersions }
```

### `match-branches.js`

```javascript
import { matchBranches } from './scripts/common/match-branches.js';

const jiraData = { ticket: 'TT-12345', fixVersions: [...] };
const branches = [{ name: 'main' }, { name: 'release-5.8' }];

const result = matchBranches(jiraData, branches);
// Returns: { ticket, summary, matchResults, markdown }
```

### `add-pr-comment.js`

```javascript
import { addOrUpdateComment } from './scripts/github/add-pr-comment.js';

const result = await addOrUpdateComment(
  'TykTechnologies',  // owner
  'tyk',              // repo
  123,                // PR number
  '## Comment body',  // markdown content
  '<!-- marker -->'   // unique identifier
);
```

## License

[Your License Here]

## Support

For issues or questions:
- Open an issue in this repository
- Contact the DevOps team
