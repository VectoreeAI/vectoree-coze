#!/bin/sh
# Read commit messages in a revision range. On a high-confidence secret
# pattern, print only the commit SHA and the pattern name. Do not print
# the message, and do not modify history.
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: scan_commit_messages.sh <revision-range>" >&2
  exit 2
fi

range=$1
status=0
shas=$(git rev-list "$range")

for sha in $shas; do
  msg=$(git log -1 --format=%B "$sha")
  kind=
  if printf '%s\n' "$msg" | grep -E -q -e '-----BEGIN ([A-Z0-9]+ )*PRIVATE KEY-----'; then
    kind=private_key
  elif printf '%s\n' "$msg" | grep -E -q '(^|[^A-Za-z0-9])(ghp_|github_pat_|gho_|ghu_|ghs_|ghr_|glpat-)'; then
    kind=token_prefix
  elif printf '%s\n' "$msg" | grep -E -q 'AKIA[0-9A-Z]{16}'; then
    kind=aws_access_key
  elif printf '%s\n' "$msg" | grep -E -q 'https?://[^[:space:]/@]+:[^[:space:]@]+@'; then
    kind=userinfo_url
  fi
  if [ -n "$kind" ]; then
    printf 'commit %s matched %s\n' "$sha" "$kind"
    status=1
  fi
done

exit "$status"
