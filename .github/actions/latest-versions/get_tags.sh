#!/usr/bin/env bash
set -euo pipefail

# GITHUB_OUTPUT='/dev/tty'
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
		repository=${repository//-/_}
		echo "$repository=$default_tag" >> $GITHUB_OUTPUT
		echo "#DEBUG# $repository=$default_tag"
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
	echo "#DEBUG# tyk=$branch"
	echo "tyk_analytics=$branch" >> $GITHUB_OUTPUT
	echo "#DEBUG# tyk_analytics=$branch"
	echo "tyk_pump=$(get_latest_tag 'tyk-pump')" >> $GITHUB_OUTPUT
	echo "#DEBUG# tyk_pump=$(get_latest_tag 'tyk-pump')"
	echo "tyk_sink=$(get_latest_tag 'tyk-sink')" >> $GITHUB_OUTPUT
	echo "#DEBUG# tyk_sink=$(get_latest_tag 'tyk-sink')"
	if [[ "$branch" =~ ^release-[0-9]-lts$ ]];then
		echo "tyk_automated_tests=$branch" >> $GITHUB_OUTPUT
		echo "#DEBUG# tyk_automated_tests=$branch"
	else
		echo "tyk_automated_tests=master" >> $GITHUB_OUTPUT
		echo "#DEBUG# tyk_automated_tests=master"
	fi
	
else #default to master case
	bulk_set_tag master
fi

# Override always with build_tag, does not contain ecr URL
current_repo=${current_repo//-/_}
echo "$current_repo=sha-$commit_sha" >> $GITHUB_OUTPUT
echo "#DEBUG# $current_repo=sha-$commit_sha"

