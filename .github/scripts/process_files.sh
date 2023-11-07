#!/bin/bash
echo "AAA"
echo $BASE_URL
if [ "$#" -ne 6 ]; then
    echo "Usage: $0 TOKEN BASE_URL PARENT_COMMIT_SHA QUERY_PARAMS API_IDS FILE_REGEX"
    exit 1
fi

TOKEN="$1"
BASE_URL="$2"
PARENT_COMMIT_SHA="$3"
QUERY_PARAMS="$4"
API_IDS="$5"
FILE_REGEX="$6"

check_api_existence() {
    local endpoint=$1
    local token=$2

    status_code=$(curl -s -o /dev/null -w "%{http_code}" -X GET -H "Authorization: $token" --location "$endpoint")
    echo $status_code
}

patch_api() {
    local endpoint=$1
    local token=$2
    local params=$3
    local data=$4

    response=$(curl -s -o /dev/null -w "%{http_code}" -X PATCH -H "Authorization: $token" -H "Content-Type: application/json" -d "$data" "${endpoint}?${params}")
    echo $response
}

import_api() {
    local endpoint=$1
    local token=$2
    local api_id=$3
    local params=$4
    local data=$5

    response=$(curl -s -o /dev/null -w "%{http_code}" -X POST -H "Authorization: $token" -H "Content-Type: application/json" -d "$data" "${endpoint}?apiID=$api_id&${params}")
    echo $response
}

echo "Start iterating over files"
git diff-tree --no-commit-id --name-only -r "$PARENT_COMMIT_SHA"
files=$(git diff-tree --no-commit-id --name-only -r "$PARENT_COMMIT_SHA" | grep -E "$FILE_REGEX")

for file in $files; do
    content=$(<"$file")
    echo "Processing file: $file"
    filename=$(basename -- "$file")
    filename="${filename%.*}"
    api_id=$(echo "$API_IDS" | jq -r ".[\"$filename\"]")

    echo "API ID: $api_id"
    query_params=$(echo "$QUERY_PARAMS" | jq -r ".[\"$filename\"]")

    importEndpoint="${BASE_URL}/api/apis/oas/import"
    endpoint="${BASE_URL}/api/apis/oas/${api_id}"

    echo "Endpoint: $endpoint"

    response=$(check_api_existence "$endpoint" "$TOKEN")

    if [ "$response" -eq 200 ]; then
        echo "API with ID $api_id already exists. Performing PATCH request."
        response=$(patch_api "$endpoint" "$TOKEN" "$query_params" "$content")
        
        if [ "$response" -eq 200 ]; then
            echo "API has been patched successfully."
        else
            echo "API patch request failed with status code $response."
        fi
    else
        echo "API with ID $api_id does not exist. Performing IMPORT request."
        response=$(import_api "$importEndpoint" "$TOKEN" "$api_id" "$query_params" "$content")
        
        if [ "$response" -eq 200 ]; then
            echo "Import of OAS was successful."
        else
            echo "Import of OAS failed with status code $response."
        fi
    fi
done
