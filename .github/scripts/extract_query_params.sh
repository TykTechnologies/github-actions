#!/bin/bash
content=$(cat "tyk-manifest.json")
declare -A params_array
declare -A api_ids_array

while IFS= read -r key; do
  api_id=$(echo "$content" | jq -r ".[\"$key\"].api_id")
  params=$(echo "$content" | jq -r ".[\"$key\"].params | to_entries | map(\"\(.key)=\(.value|tostring)\") | join(\"&\")")

  params_array["$key"]="$params"
  api_ids_array["$key"]="$api_id"
done < <(jq -r 'keys[]' "tyk-manifest.json")

json_params="{"
json_api_ids="{" 

for key in "${!params_array[@]}"; do
  json_params+="\"$key\":\"${params_array[$key]}\","
  json_api_ids+="\"$key\":\"${api_ids_array[$key]}\","
done

json_params=${json_params%,}
json_params+="}"
json_api_ids=${json_api_ids%,}
json_api_ids+="}"

echo "queryParams=${json_params}" >> $GITHUB_OUTPUT
echo "apiIds=${json_api_ids}" >> $GITHUB_OUTPUT
