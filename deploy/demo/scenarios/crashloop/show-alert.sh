#!/usr/bin/env bash
# Query AlertManager for the KubePodCrashLooping alert and display it.
set -euo pipefail

ALERT_NAME="${1:-KubePodCrashLooping}"
AM_POD="alertmanager-kube-prometheus-stack-alertmanager-0"

ALERTS_JSON=$(kubectl exec -n monitoring "$AM_POD" -- \
  amtool alert query "alertname=$ALERT_NAME" \
  --alertmanager.url=http://localhost:9093 \
  --output=json 2>/dev/null)

COUNT=$(echo "$ALERTS_JSON" | python3 -c "import sys,json; a=json.load(sys.stdin); print(len(a))" 2>/dev/null || echo "0")

printf '\n'
printf '  ┌─────────────────────────────────────────────────────────┐\n'
printf '  │  AlertManager: Active Alerts                            │\n'
printf '  └─────────────────────────────────────────────────────────┘\n'
printf '\n'

if [ "$COUNT" = "0" ]; then
  printf '  No active alerts matching "%s"\n' "$ALERT_NAME"
else
  echo "$ALERTS_JSON" | python3 -c "
import sys, json
alerts = json.load(sys.stdin)
for a in alerts:
    labels = a.get('labels', {})
    annots = a.get('annotations', {})
    state  = a.get('status', {}).get('state', 'unknown')
    print(f\"  Alert:      {labels.get('alertname', 'N/A')}\")
    print(f\"  Severity:   {labels.get('severity', 'N/A')}\")
    print(f\"  State:      {state}\")
    print(f\"  Namespace:  {labels.get('namespace', 'N/A')}\")
    print(f\"  Pod:        {labels.get('pod', 'N/A')}\")
    print(f\"  Container:  {labels.get('container', 'N/A')}\")
    summary = annots.get('summary', '')
    if summary:
        print(f\"  Summary:    {summary}\")
    print()
" 2>/dev/null
fi
