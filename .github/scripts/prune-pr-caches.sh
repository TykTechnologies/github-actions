#!/usr/bin/env bash
# Prune GitHub Actions caches that belong to pull-request refs (refs/pull/<N>/merge).
#
# Only PR refs are ever touched. master, release branches and tags are never listed for
# deletion. For each PR ref:
#   closed/merged PR  -> delete all its caches
#   open PR           -> keep the newest entries up to BUDGET_MB, always keep entries younger
#                        than GUARD_MIN minutes (a running job may be using them), delete the rest
#   unknown state     -> keep
# Dry run unless DRY_RUN=false.
#
# Environment: GH_TOKEN (needs actions:write and pull_requests:read on the repositories),
# REPOS (space separated names), OWNER, DRY_RUN, BUDGET_MB, GUARD_MIN, MAX_DELETES (per repo).
set -euo pipefail

OWNER="${OWNER:-TykTechnologies}"
REPOS="${REPOS:?REPOS is required, e.g. \"tyk tyk-analytics\"}"
DRY_RUN="${DRY_RUN:-true}"
BUDGET_MB="${BUDGET_MB:-1024}"
GUARD_MIN="${GUARD_MIN:-30}"
MAX_DELETES="${MAX_DELETES:-500}"
SUMMARY="${GITHUB_STEP_SUMMARY:-/dev/null}"

budget_bytes=$((BUDGET_MB * 1024 * 1024))
guard_seconds=$((GUARD_MIN * 60))
now=$(date -u +%s)

mode="DRY RUN (nothing is deleted)"
if [ "$DRY_RUN" = "false" ]; then mode="DELETE"; fi
echo "Mode: $mode, budget per open PR: ${BUDGET_MB} MB, guard: ${GUARD_MIN} min"
{
  echo "## PR cache pruning: $mode"
  echo
  echo "Budget per open PR: ${BUDGET_MB} MB, guard: ${GUARD_MIN} min."
  echo
  echo "| repo | PR refs (closed/open/unknown) | PR caches | to delete | freed |"
  echo "|---|---|---|---|---|"
} >> "$SUMMARY"

# Decides per cache entry. Input: array of {id, ref, size, created_at}.
# shellcheck disable=SC2016  # a jq program, the $variables are jq variables
decide='
def age: $now - (.created_at | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601);
group_by(.ref)
| map(
    (.[0].ref | capture("^refs/pull/(?<n>[0-9]+)/merge$").n) as $n
    | ($states[$n] // "unknown") as $st
    | sort_by(.created_at) | reverse
    | [foreach .[] as $e (0; . + $e.size; $e + {cum: .})]
    | map(. + {pr: ($n | tonumber), state: $st,
        action: (if $st == "closed" then "delete"
                 elif $st == "open" and (age >= $guard) and ((.cum - .size) >= $budget) then "delete"
                 else "keep" end)})
  )
| flatten
'

total_deleted=0
for repo in $REPOS; do
  echo "== $OWNER/$repo"
  if ! caches=$(gh api --paginate "repos/$OWNER/$repo/actions/caches?per_page=100" \
      --jq '.actions_caches[] | select(.ref | test("^refs/pull/[0-9]+/merge$")) | {id, ref, size: .size_in_bytes, created_at}' \
      | jq -s '.'); then
    echo "::warning::cannot list caches of $repo, skipping"
    continue
  fi

  states='{}'
  for n in $(echo "$caches" | jq -r '[.[].ref | capture("^refs/pull/(?<n>[0-9]+)/merge$").n] | unique | .[]'); do
    st=$(gh api "repos/$OWNER/$repo/pulls/$n" --jq '.state' 2>/dev/null || echo unknown)
    states=$(jq -n --argjson s "$states" --arg n "$n" --arg v "$st" '$s + {($n): $v}')
  done

  decisions=$(echo "$caches" | jq --argjson states "$states" --argjson now "$now" \
    --argjson guard "$guard_seconds" --argjson budget "$budget_bytes" "$decide")

  line=$(echo "$decisions" | jq -r --argjson states "$states" --argjson max "$MAX_DELETES" --arg repo "$repo" '
    ([$states | to_entries[] | select(.value=="closed")] | length) as $c
    | ([$states | to_entries[] | select(.value=="open")] | length) as $o
    | ([$states | to_entries[] | select(.value!="closed" and .value!="open")] | length) as $u
    | ([.[] | select(.action=="delete")] | .[0:$max]) as $del
    | "| \($repo) | \($c)/\($o)/\($u) | \((map(.size)|add // 0)/1073741824*100|floor/100) GB (\(length)) | \($del|length) | \(($del|map(.size)|add // 0)/1073741824*100|floor/100) GB |"')
  echo "$line" >> "$SUMMARY"
  echo "$line"

  echo "$decisions" | jq -r '
    map(select(.action=="delete")) | group_by(.pr)
    | map({pr: .[0].pr, state: .[0].state, n: length, mb: ((map(.size)|add)/1048576|floor)})
    | sort_by(-.mb) | .[0:8][] | "   PR #\(.pr) (\(.state)): delete \(.n) entries, \(.mb) MB"'

  ids=$(echo "$decisions" | jq -r --argjson max "$MAX_DELETES" '[.[] | select(.action=="delete")] | .[0:$max][].id')
  count=0
  failed=0
  for id in $ids; do
    count=$((count + 1))
    if [ "$DRY_RUN" = "false" ]; then
      gh api -X DELETE "repos/$OWNER/$repo/actions/caches/$id" >/dev/null 2>&1 || failed=$((failed + 1))
    fi
  done
  total_deleted=$((total_deleted + count))
  if [ "$DRY_RUN" = "false" ] && [ "$failed" -gt 0 ]; then
    echo "::warning::$repo: $failed of $count deletions failed"
  fi
done

echo "Entries selected for deletion: $total_deleted ($mode)"
