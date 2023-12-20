#!/usr/bin/env bash
set -euo pipefail

REPO_LIST="tyk tyk-analytics tyk-pump tyk-sink"

get_tag() {
 tyk_repo=${1}
 git -c 'versionsort.suffix=-' ls-remote --exit-code --refs --sort='version:refname' --tags https://github.com/TykTechnologies/${tyk_repo}.git '*.*.*' | egrep 'v[0-9]+.[0-9]+.[0-9]+$' | tail -1 | cut -d '/' -f3
}

for repo in ${REPO_LIST};do
	version=$(get_tag ${repo})
	echo "$repo=$version"
	echo "$repo=$version" >> $GITHUB_OUTPUT
done