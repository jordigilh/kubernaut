#!/usr/bin/env bash
# Deprecated application-chart validation retained for offline checks only.
# Issue #2492: live qualification uses kubernaut-operator bootstrap + Kubernaut CR.
set -euo pipefail

CHART_PATH="charts/kubernaut"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --chart-path) CHART_PATH="$2"; shift 2 ;;
    -h|--help) echo "Usage: $0 [--chart-path PATH]"; exit 0 ;;
    *) shift ;;
  esac
done

command -v helm >/dev/null || { echo "helm is required for offline deprecated-chart validation" >&2; exit 1; }
helm lint "$CHART_PATH" --strict
helm template kubernaut "$CHART_PATH" >/tmp/deprecated-kubernaut-rendered.yaml
test -s /tmp/deprecated-kubernaut-rendered.yaml
echo "Deprecated application chart lint/render passed; no cluster or workload was created."
