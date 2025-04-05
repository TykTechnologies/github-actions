#!/usr/bin/env bash
set -e

# Inputs and environment variables
RUN_ID="${TARGET_RUN_ID:-$GITHUB_RUN_ID}"
REPO="${TARGET_REPO:-$GITHUB_REPOSITORY}"

# Extract branch name - prioritize base_ref for PRs
if [[ -n "$GITHUB_BASE_REF" ]]; then
  # For Pull Requests, use the destination branch directly
  BRANCH="$GITHUB_BASE_REF"
elif [[ -n "$GITHUB_REF" ]]; then
  # For direct pushes, extract from refs/heads/branch-name
  BRANCH=$(echo "$GITHUB_REF" | sed -e 's,.*/\(.*\),\1,')
else
  # Default to empty string if not available
  BRANCH=""
fi

echo "Branch: $BRANCH"

# 1. List all jobs for the workflow run and filter for failures
echo "Fetching jobs for run $RUN_ID in $REPO..."
JOBS_JSON=$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
                 -H "Accept: application/vnd.github+json" \
                 "https://api.github.com/repos/$REPO/actions/runs/$RUN_ID/jobs?per_page=100")

# Use jq to find failed jobs and their first failed step (if any)
FAILED_JOBS=$(echo "$JOBS_JSON" | jq -c '.jobs[] | select(.conclusion=="failure") | {id: .id, name: .name, completed_at: .completed_at, failed_step: ((.steps[]? | select(.conclusion=="failure") | .name) // null)}')

# 2. Loop through each failed job and handle its log
echo "$FAILED_JOBS" | while IFS= read -r job; do
  [ -z "$job" ] && continue  # skip if empty
  job_id=$(echo "$job" | jq -r '.id')
  job_name=$(echo "$job" | jq -r '.name')
  step_name=$(echo "$job" | jq -r '.failed_step')
  timestamp=$(echo "$job" | jq -r '.completed_at')

  echo "Downloading log for failed job '$job_name' (ID $job_id)..."
  # Download the job log (the API returns a redirect to a text log file)
  curl -s -L -H "Authorization: Bearer $GITHUB_TOKEN" \
       "https://api.github.com/repos/$REPO/actions/jobs/$job_id/logs" \
       -o "job_${job_id}.log"

  # Preprocess the log to reduce size
  echo "Preprocessing log for job '$job_name'..."
  
  # Remove ANSI color codes
  sed -i 's/\x1b\[[0-9;]*m//g' "job_${job_id}.log"
  
  # Remove timestamp patterns (optional, but helps reduce size)
  sed -i -E 's/^\[?[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})\]? //g' "job_${job_id}.log"
  
  # Remove debug/trace lines (optional)
  grep -v "^DEBUG:" "job_${job_id}.log" | grep -v "^TRACE:" > "preprocessed_${job_id}.log" || cp "job_${job_id}.log" "preprocessed_${job_id}.log"
  
  # 3. Format the JSON payload with required fields
  payload=$(jq -n \
    --arg repo "$REPO" \
    --arg run_id "$RUN_ID" \
    --arg job_name "$job_name" \
    --arg step_name "${step_name:-}" \
    --arg branch "$BRANCH" \
    --arg timestamp "$timestamp" \
    --rawfile raw_log "preprocessed_${job_id}.log" \
    '{ repo: $repo, run_id: $run_id, job_name: $job_name, step_name: $step_name, branch: $branch, timestamp: $timestamp, raw_log: $raw_log }')

  # 4. Send the JSON to the external API with Authorization header using GH_LOGS_ANALYSER secret
  echo "Sending log for '$job_name' (step '$step_name', branch '$BRANCH')..."
  curl -s -X POST \
       -H "Content-Type: application/json" \
       -H "Authorization: Bearer $GH_LOGS_ANALYSER" \
       -d "$payload" "https://d61b-81-18-84-142.ngrok-free.app/api/v1/logs" || \
       echo "Warning: Failed to send log to API."

done

echo "Log extraction completed."
