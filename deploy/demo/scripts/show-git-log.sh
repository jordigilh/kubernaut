#!/usr/bin/env bash
# Show recent git commits from a Gitea repository via the API.
# Usage: show-git-log.sh [limit]
set -euo pipefail

LIMIT="${1:-5}"
REPO="kubernaut/demo-gitops-repo"
GITEA_NS="gitea"

kubectl port-forward -n "${GITEA_NS}" svc/gitea-http 3000:3000 &>/dev/null &
PF_PID=$!
trap 'kill ${PF_PID} 2>/dev/null || true' EXIT
sleep 2

COMMITS=$(curl -sf "http://localhost:3000/api/v1/repos/${REPO}/commits?limit=${LIMIT}" 2>/dev/null || echo "[]")

if ! command -v jq &>/dev/null || [ "$COMMITS" = "[]" ]; then
  echo "(no commits or Gitea unavailable)"
  exit 0
fi

echo "$COMMITS" | jq -r '.[] | "\(.sha[0:7]) \(.commit.message | split("\n")[0])"'
