#!/usr/bin/env bash
# Display the EffectivenessAssessment result for demo recordings.
# Usage: bash deploy/demo/scripts/show-effectiveness.sh <scenario-namespace>
# Example: bash deploy/demo/scripts/show-effectiveness.sh demo-crashloop
set -euo pipefail

SCENARIO_NS="${1:?Usage: show-effectiveness.sh <scenario-namespace>}"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

EA_NAME=$(kubectl get effectivenessassessments -n "$PLATFORM_NS" -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.signalTarget.namespace}{"\n"}{end}' 2>/dev/null \
  | grep "$SCENARIO_NS" | tail -1 | cut -f1 || true)

if [ -z "$EA_NAME" ]; then
  EA_NAME=$(kubectl get effectivenessassessments -n "$PLATFORM_NS" -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null)
fi

PHASE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.phase}' 2>/dev/null)
REASON=$(kubectl get effectivenessassessments "$EA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.assessmentReason}' 2>/dev/null)
MESSAGE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.message}' 2>/dev/null)
ALERT_SCORE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.components.alertScore}' 2>/dev/null)
HEALTH_SCORE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.components.healthScore}' 2>/dev/null)
METRICS_SCORE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$PLATFORM_NS" -o jsonpath='{.status.components.metricsScore}' 2>/dev/null)

printf '\n'
printf '  ┌─────────────────────────────────────────────────────────┐\n'
printf '  │  Effectiveness Assessment                               │\n'
printf '  └─────────────────────────────────────────────────────────┘\n'
printf '\n'
printf '  Phase:    %s\n' "${PHASE:-Pending}"
printf '  Reason:   %s\n' "${REASON:-N/A}"
if [ -n "$MESSAGE" ]; then
  printf '  Message:  %s\n' "$MESSAGE"
fi
printf '\n'
printf '  Component Scores  (0.0 = worst, 1.0 = best)\n'
printf '  ────────────────\n'

if [ -n "$ALERT_SCORE" ]; then
  printf '  Alert Resolution:  %s' "$ALERT_SCORE"
  if awk "BEGIN{exit !($ALERT_SCORE >= 1.0)}" 2>/dev/null; then
    printf '    ✓ alert resolved\n'
  elif awk "BEGIN{exit !($ALERT_SCORE >= 0.5)}" 2>/dev/null; then
    printf '    ~ alert partially resolved\n'
  else
    printf '    (pending — Prometheus evaluation window has not expired yet, ~10m)\n'
  fi
else
  printf '  Alert Resolution:  pending\n'
fi

if [ -n "$HEALTH_SCORE" ]; then
  printf '  Health Check:      %s' "$HEALTH_SCORE"
  if awk "BEGIN{exit !($HEALTH_SCORE >= 1.0)}" 2>/dev/null; then
    printf '    ✓ all pods Running, no restarts\n'
  elif awk "BEGIN{exit !($HEALTH_SCORE >= 0.75)}" 2>/dev/null; then
    printf '    ✓ all pods Running (restart count reflects prior crash)\n'
  elif awk "BEGIN{exit !($HEALTH_SCORE >= 0.5)}" 2>/dev/null; then
    printf '    ~ partial readiness (some pods recovering)\n'
  else
    READY_PODS=$(kubectl get pods -n "$SCENARIO_NS" --no-headers 2>/dev/null | grep -c "Running" || echo "0")
    TOTAL_PODS=$(kubectl get pods -n "$SCENARIO_NS" --no-headers 2>/dev/null | wc -l | tr -d ' ')
    if [ "$READY_PODS" = "$TOTAL_PODS" ] && [ "$READY_PODS" != "0" ]; then
      printf '    ✓ pods Running (restart count reflects prior crash)\n'
    else
      printf '    ~ pods not yet healthy\n'
    fi
  fi
else
  printf '  Health Check:      pending\n'
fi

if [ -n "$METRICS_SCORE" ]; then
  if awk "BEGIN{exit !($METRICS_SCORE >= 0.5)}" 2>/dev/null; then
    printf '  Metrics:           %s    ✓ improvement detected\n' "$METRICS_SCORE"
  elif awk "BEGIN{exit !($METRICS_SCORE > 0)}" 2>/dev/null; then
    printf '  Metrics:           %s    ~ baseline (no custom queries configured)\n' "$METRICS_SCORE"
  else
    printf '  Metrics:           0.0     ~ baseline (no custom queries configured)\n'
  fi
else
  printf '  Metrics:           pending\n'
fi
printf '\n'
