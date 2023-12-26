#!/usr/bin/env bash
set -euo pipefail

#GITHUB_OUTPUT='/dev/tty'
REPO_LIST=("tyk" "tyk-analytics" "tyk-pump" "tyk-sink" "tyk-automated-tests")

current_repo=${1##*/}
event=${2}
ref=${3}
base_ref=${4}
commit_sha=${5}

function get_latest_tag() {
 tyk_repo=${1}
 git -c 'versionsort.suffix=-' ls-remote --exit-code --refs --sort='version:refname' --tags https://github.com/TykTechnologies/${tyk_repo}.git '*.*.*' | egrep 'v[0-9]+.[0-9]+.[0-9]+$' | tail -1 | cut -d '/' -f3
}

function bulk_set_tag() {
	default_tag=${1}
	for repository in "${REPO_LIST[@]}";do
		echo "$repository=$default_tag" >> $GITHUB_OUTPUT
	done
}

if [ $event == 'pull_request' ];then
	branch=${base_ref}
else
	branch=${ref##*/}
fi

# release-x-case = branch
# optional to only accept release-x-lts cases use ^release-[0-9]-lts$
if [ "$current_repo" == 'tyk' -o "$current_repo" == 'tyk-analytics' ] && [[ "$branch" =~ ^release- ]]; then 
	echo "tyk=$branch" >> $GITHUB_OUTPUT
	echo "tyk-analytics=$branch" >> $GITHUB_OUTPUT
	echo "tyk-pump=$(get_latest_tag 'tyk-pump')" >> $GITHUB_OUTPUT
	echo "tyk-sink=$(get_latest_tag 'tyk-sink')" >> $GITHUB_OUTPUT
	if [[ "$branch" =~ ^release-[0-9]-lts$ ]];then
		echo "tyk-automated-tests=$branch" >> $GITHUB_OUTPUT
	else
		echo "tyk-automated-tests=master" >> $GITHUB_OUTPUT
	fi
	
else #default to master case
	bulk_set_tag master
fi

# Override always with build_tag, does not contain ecr URL
echo "$current_repo=sha-$commit_sha" >> $GITHUB_OUTPUT

