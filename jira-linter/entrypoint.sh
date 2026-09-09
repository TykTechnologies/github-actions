#!/bin/sh
# Entrypoint for the jira-linter Docker container action.
#
# A Docker container action is a single step, so everything the previous
# composite action spread across several steps happens here: resolve the
# branch name, run the linter, and post or clear the sticky PR comment.

set -u

STDERR_FILE=/tmp/stderr.txt
COMMENT_FILE=/tmp/linter-output.txt
COMMENT_JSON=/tmp/comment.json

MARKER='<!-- jira-linter-comment-marker -->'
# Marker written by marocchino/sticky-pull-request-comment, which this action
# used previously. Matched as well so comments already on open PRs are reused
# or removed instead of being left behind as duplicates.
LEGACY_MARKER='<!-- Sticky Pull Request Commentjira-linter -->'

API_URL="${GITHUB_API_URL:-https://api.github.com}"
SERVER_URL="${GITHUB_SERVER_URL:-https://github.com}"
REPOSITORY="${GITHUB_REPOSITORY:-}"
TOKEN="${GITHUB_TOKEN:-}"
PR_NUMBER="${JL_PR_NUMBER:-}"

# ---- GitHub API helpers -----------------------------------------------------

gh_api() {
  method="$1"
  url="$2"
  shift 2

  curl --silent --show-error --fail-with-body \
    --request "$method" \
    --header "Authorization: Bearer $TOKEN" \
    --header 'Accept: application/vnd.github+json' \
    --header 'X-GitHub-Api-Version: 2022-11-28' \
    "$@" "$url"
}

# comments_enabled reports whether there is enough context to talk to the
# issue comments API. Running the linter locally, or on a non-PR event, is
# still valid: the validation result just is not mirrored to a comment.
comments_enabled() {
  [ -n "$TOKEN" ] && [ -n "$REPOSITORY" ] && [ -n "$PR_NUMBER" ]
}

# find_sticky_comment_id prints the id of the most recent jira-linter comment
# on the PR, or nothing when there is none.
find_sticky_comment_id() {
  page=1
  found=''

  # Cap the walk so a pathologically long PR thread cannot spin forever.
  while [ "$page" -le 10 ]; do
    payload=$(gh_api GET "$API_URL/repos/$REPOSITORY/issues/$PR_NUMBER/comments?per_page=100&page=$page") || return 1

    id=$(printf '%s' "$payload" | jq -r --arg m "$MARKER" --arg l "$LEGACY_MARKER" \
      'map(select(.body | contains($m) or contains($l))) | last | .id // empty')
    if [ -n "$id" ]; then
      found="$id"
    fi

    count=$(printf '%s' "$payload" | jq 'length')
    if [ "$count" -lt 100 ]; then
      break
    fi
    page=$((page + 1))
  done

  printf '%s' "$found"
}

delete_sticky_comments() {
  comments_enabled || return 0

  while :; do
    id=$(find_sticky_comment_id) || return 0
    [ -n "$id" ] || return 0

    gh_api DELETE "$API_URL/repos/$REPOSITORY/issues/comments/$id" >/dev/null \
      || return 0
    echo "--> removed stale jira-linter comment $id"
  done
}

# post_sticky_comment deletes any existing jira-linter comment before creating
# the new one, matching the `recreate: true` behaviour of the sticky comment
# action so the latest result always sits at the bottom of the thread.
post_sticky_comment() {
  if ! comments_enabled; then
    echo "--> skipping PR comment: GITHUB_TOKEN, GITHUB_REPOSITORY or JL_PR_NUMBER is not set" >&2
    return 0
  fi

  delete_sticky_comments

  jq -Rs '{body: .}' < "$COMMENT_FILE" > "$COMMENT_JSON"
  gh_api POST "$API_URL/repos/$REPOSITORY/issues/$PR_NUMBER/comments" \
    --header 'Content-Type: application/json' \
    --data "@$COMMENT_JSON" >/dev/null \
    || echo "--> failed to post the jira-linter comment" >&2
}

write_comment_body() {
  commit_hash="${HEAD_SHA:-${GITHUB_SHA:-}}"
  commit_short=$(printf '%s' "$commit_hash" | cut -c1-7)
  timestamp=$(date -u '+%Y-%m-%d %H:%M:%S UTC')

  cat > "$COMMENT_FILE" <<EOF
## 🚨 Jira Linter Failed

**Commit:** [\`${commit_short}\`](${SERVER_URL}/${REPOSITORY}/commit/${commit_hash})
**Failed at:** ${timestamp}

The Jira linter failed to validate your PR. Please check the error details below:

<details>
<summary>🔍 Click to view error details</summary>

\`\`\`
$(cat "$STDERR_FILE")
\`\`\`

</details>

### Next Steps
- Ensure your branch name contains a valid Jira ticket ID (e.g., \`ABC-123\`)
- Verify your PR title matches the branch's Jira ticket ID
- Check that the Jira ticket exists and is accessible

---
_This comment will be automatically deleted once the linter passes._

${MARKER}
EOF
}

# ---- Run --------------------------------------------------------------------

# The branch name is passed as the first argument by action.yaml. The
# GITHUB_HEAD_REF -> GITHUB_REF_NAME fallback is repeated here so a direct
# `docker run` without arguments behaves the same way.
BRANCH_NAME="${1:-}"
if [ -z "$BRANCH_NAME" ]; then
  BRANCH_NAME="${GITHUB_HEAD_REF:-${GITHUB_REF_NAME:-}}"
fi

if [ -z "$BRANCH_NAME" ]; then
  echo "unable to resolve a branch name from the argument, GITHUB_HEAD_REF or GITHUB_REF_NAME" >&2
  exit 1
fi

echo "--> running the linter against $BRANCH_NAME branch"

# Capture stderr for the failure comment, then replay it so it still reaches
# the workflow log.
linter --branch="$BRANCH_NAME" 2> "$STDERR_FILE"
STATUS=$?

if [ -s "$STDERR_FILE" ]; then
  cat "$STDERR_FILE" >&2
fi

if [ "$STATUS" -eq 0 ]; then
  delete_sticky_comments
  exit 0
fi

write_comment_body
post_sticky_comment

echo "❌ Jira Linter failed - failing the action"
exit "$STATUS"
