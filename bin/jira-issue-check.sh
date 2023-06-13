#!/bin/bash

# Get the branch name from GITHUB_REF
BRANCH_NAME=$(basename "${GITHUB_HEAD_REF}")

# Extract JIRA issue number from branch name
BRANCH_ISSUE_NUMBER=$(echo "${BRANCH_NAME}" | grep -o 'TT-[0-9]\+' || true)

# Extract JIRA issue number from pull request title
PR_ISSUE_NUMBER=$(echo "${PULL_REQUEST_TITLE}" | grep -o 'TT-[0-9]\+' || true)
# Check if JIRA issue number is present in either the branch name or the pull request title
# Get the changed files from the GitHub event payload
CHANGED_FILES=$(jq -r '.pull_request.changed_files' "$GITHUB_EVENT_PATH")

# Check if changes include only GitHub workflow files or bin folder
if echo "$CHANGED_FILES" | grep -vqE '.*\.github/workflows/.*\.ya?ml$|.*bin/.*'; then
  # Continue with the JIRA issue number check
  if [ -z "$BRANCH_ISSUE_NUMBER" ] && [ -z "$PR_ISSUE_NUMBER" ]; then
    echo "JIRA issue number is missing in both the branch name and the pull request title."
    echo "valid=false" >> $GITHUB_OUTPUT

    # Comment on the pull request with the error message
    echo "Commenting on the pull request with the error message..."
    COMMENT_BODY="JIRA issue number is missing in the branch name and the pull request title. Please include the JIRA issue number in either the branch name or the pull request title."
    PAYLOAD=$(echo '{}' | jq --arg body "$COMMENT_BODY" '.body = $body')
    URL=$(jq -r .pull_request.comments_url "$GITHUB_EVENT_PATH")
    curl -s -X POST -H "Authorization: token $TOKEN" -d "$PAYLOAD" "$URL"

    exit 1
  else
    echo "JIRA issue number is present in either the branch name or the pull request title."

    if [ -n "$BRANCH_ISSUE_NUMBER" ]; then
      echo "JIRA issue number $BRANCH_ISSUE_NUMBER is present in the branch name."
      echo "jira_issue_number=$BRANCH_ISSUE_NUMBER" >> $GITHUB_OUTPUT
    else
      echo "JIRA issue number $PR_ISSUE_NUMBER is present in the pull request title."
      echo "jira_issue_number=$PR_ISSUE_NUMBER" >> $GITHUB_OUTPUT
    fi
  fi

  echo "valid=true" >> $GITHUB_OUTPUT
else
  echo "Changes to GitHub workflow files or the bin folder detected. Skipping JIRA issue number check."
  echo "valid=true" >> $GITHUB_OUTPUT
fi