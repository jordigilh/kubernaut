#!/usr/bin/env bash
# Show NotificationRequest CRDs for demo scenarios
# Usage: show-notification.sh <namespace> [name-pattern]
set -euo pipefail

NAMESPACE="${1:?Usage: show-notification.sh <namespace> [name-pattern]}"
NAME_PATTERN="${2:-}"

# Fetch NotificationRequest CRDs
JSON=$(kubectl get notificationrequest -n "$NAMESPACE" -o json 2>/dev/null || echo '{"items":[]}')

# Count items
COUNT=0
if command -v jq &>/dev/null; then
  COUNT=$(echo "$JSON" | jq '.items | length // 0')
else
  COUNT=$(kubectl get notificationrequest -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null | wc -w | tr -d ' ')
fi

# No NotificationRequest found
if [ "${COUNT:-0}" -eq 0 ]; then
  printf '\n'
  printf '╔══════════════════════════════════════════════════════╗\n'
  printf '║  No notification generated -- pipeline completed   ║\n'
  printf '║  without action                                     ║\n'
  printf '╚══════════════════════════════════════════════════════╝\n'
  printf '\n'
  exit 0
fi

# Display one NotificationRequest (reads JSON from stdin or uses name for kubectl)
display_one() {
  local subj body prio type channels meta name
  local use_jq=false
  if command -v jq &>/dev/null; then
    use_jq=true
  fi

  if $use_jq; then
    local json
    json=$(cat)
    subj=$(echo "$json" | jq -r '.spec.subject // ""')
    body=$(echo "$json" | jq -r '.spec.body // ""')
    prio=$(echo "$json" | jq -r '.spec.priority // ""')
    type=$(echo "$json" | jq -r '.spec.type // ""')
    channels=$(echo "$json" | jq -r '(.spec.channels // []) | join(", ")')
    meta=$(echo "$json" | jq -r '(.spec.metadata // {}) | to_entries | map("\(.key)=\(.value)") | join(", ")')
  else
    name="$1"
    [ -z "$name" ] && return
    subj=$(kubectl get notificationrequest "$name" -n "$NAMESPACE" -o jsonpath='{.spec.subject}' 2>/dev/null || true)
    body=$(kubectl get notificationrequest "$name" -n "$NAMESPACE" -o jsonpath='{.spec.body}' 2>/dev/null || true)
    prio=$(kubectl get notificationrequest "$name" -n "$NAMESPACE" -o jsonpath='{.spec.priority}' 2>/dev/null || true)
    type=$(kubectl get notificationrequest "$name" -n "$NAMESPACE" -o jsonpath='{.spec.type}' 2>/dev/null || true)
    channels=$(kubectl get notificationrequest "$name" -n "$NAMESPACE" -o jsonpath='{.spec.channels[*]}' 2>/dev/null | tr ' ' ',' || true)
    meta=$(kubectl get notificationrequest "$name" -n "$NAMESPACE" -o jsonpath='{.spec.metadata}' 2>/dev/null | sed 's/map\[//;s/\]//' || true)
  fi

  # Title from subject or type
  local title="${subj:-$type}"
  [ -z "$title" ] && title="Notification"

  printf '\n'
  printf '╔══════════════════════════════════════════════════════╗\n'
  printf '║  NOTIFICATION: %-36s ║\n' "${title:0:36}"
  printf '╠══════════════════════════════════════════════════════╣\n'
  printf '║  Subject:  %-42s ║\n' "${subj:0:42}"
  printf '║  Priority: %-41s ║\n' "${prio:0:41}"
  printf '║  Type:     %-41s ║\n' "${type:0:41}"
  printf '║  Channel:  %-41s ║\n' "${channels:0:41}"
  if [ -n "$meta" ]; then
    printf '║  Metadata: %-41s ║\n' "${meta:0:41}"
  fi
  printf '╠══════════════════════════════════════════════════════╣\n'
  printf '║  Body:                                               ║\n'

  # Wrap body lines to fit inside box (width 48 chars for content)
  local line_len=48
  echo "$body" | fold -s -w "$line_len" | while IFS= read -r line; do
    printf '║  %-48s ║\n' "$line"
  done

  printf '╚══════════════════════════════════════════════════════╝\n'
  printf '\n'
}

if command -v jq &>/dev/null; then
  # Filter by name pattern if provided
  if [ -n "$NAME_PATTERN" ]; then
    ITEMS=$(echo "$JSON" | jq -c --arg pat "$NAME_PATTERN" '.items[] | select(.metadata.name | contains($pat))')
  else
    ITEMS=$(echo "$JSON" | jq -c '.items[]')
  fi
  echo "$ITEMS" | while IFS= read -r item; do
    [ -n "$item" ] && echo "$item" | display_one
  done
else
  # jsonpath fallback: get names and fetch each
  for name in $(kubectl get notificationrequest -n "$NAMESPACE" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
    [ -z "$name" ] && continue
    if [ -n "$NAME_PATTERN" ] && [[ "$name" != *"$NAME_PATTERN"* ]]; then
      continue
    fi
    display_one "$name"
  done
fi
