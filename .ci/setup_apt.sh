#!/usr/bin/env bash
set -eo pipefail

if [[ -z "$1" ]]; then
  echo "Error: AIRLOCK_AR_PROJECT must be passed as the first argument." >&2
  exit 1
fi

readonly AIRLOCK_AR_PROJECT="$1"
readonly TOKEN_FILE="/workspace/.airlock_token"

if [[ ! -f "$TOKEN_FILE" ]]; then
  echo "Error: Token file $TOKEN_FILE not found." >&2
  exit 1
fi

readonly TOKEN=$(cat "$TOKEN_FILE")

source /etc/os-release
if [[ "$ID" == "ubuntu" ]]; then
  REPO="ubuntu-${VERSION_CODENAME}-3p-trusted"
else
  REPO="standard-debian-${VERSION_CODENAME}-3p-l1"
fi

echo "machine us-apt.pkg.dev login oauth2accesstoken password ${TOKEN}" > /etc/apt/auth.conf
rm -f /etc/apt/sources.list.d/* /etc/apt/sources.list
echo "deb [trusted=yes] https://us-apt.pkg.dev/projects/${AIRLOCK_AR_PROJECT} ${REPO} main" > /etc/apt/sources.list
