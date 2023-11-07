#!/bin/bash
TOKEN=$1
BASE_URL=$2

echo "TOKEN=${TOKEN}" >> $GITHUB_ENV
echo "BASE_URL=${BASE_URL}" >> $GITHUB_ENV
echo "FILE_REGEX=*.oas.json$" >> $GITHUB_ENV
