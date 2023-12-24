#!/usr/bin/env bash
set -euo pipefail

repos=("tyk" "tyk-analytics" "tyk-pump" "tyk-sink")
actions=("pull_request" "push")

# Set branches for each action
branches_pull_request=("master" "release-5-lts" "release-4-lts" "release-2.3")
branches_push=("ref/head/master" "ref/head/release-5-lts" "ref/head/release-4-lts" "ref/head/release-4.2")

function process_combination {
    local repo=$1
    local action=$2
    local ref=$3
    local base_ref=$4

    if [[ -z "$ref" ]]; then
        branch="$base_ref"
    else
        branch="$ref"
    fi

    # Your logic here
    echo -e "### Processing combination: Repo=$repo, Action=$action, Branch=$branch ### \n"
    # Modify this line according to your needs
    bash get_tags.sh "$repo" "$action" "$ref" "$base_ref" 'my_build_sha'
    echo -e "---------------------------------------------------------------------\n"
}

for repo in "${repos[@]}"; do
    for action in "${actions[@]}"; do
        if [[ "$action" == "pull_request" ]]; then
            base_refs=("${branches_pull_request[@]}")
        elif [[ "$action" == "push" ]]; then
            refs=("${branches_push[@]}")
        else
            echo "Unknown action: $action"
            exit 1
        fi

        for base_ref in "${base_refs[@]}"; do
            for ref in "${refs[@]}"; do
                process_combination "$repo" "$action" "$ref" "$base_ref"
            done
        done
    done
done
