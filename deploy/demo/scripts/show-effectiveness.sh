#!/usr/bin/env bash
# Display the EffectivenessAssessment result for demo recordings.
# Usage: bash deploy/demo/scripts/show-effectiveness.sh <scenario-namespace>
# Example: bash deploy/demo/scripts/show-effectiveness.sh demo-crashloop
set -euo pipefail

SCENARIO_NS="${1:?Usage: show-effectiveness.sh <scenario-namespace>}"
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

EA_NAME=$(kubectl get effectivenessassessments -n "$PLATFORM_NS" -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.signalTarget.namespace}{"\n"}{end}' 2>/dev/null \
  | grep "$SCENARIO_NS" | tail -1 | cut -f1)

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
  if [ "$ALERT_SCORE" = "1" ]; then
    printf '    -- alert is no longer firing\n'
  else
    printf '    -- alert is still active\n'
  fi
else
  printf '  Alert Resolution:  pending\n'
fi

if [ -n "$HEALTH_SCORE" ]; then
  printf '  Health Check:      %s' "$HEALTH_SCORE"
  if [ "$HEALTH_SCORE" = "1" ]; then
    printf '    -- all pods Running, desired replicas available\n'
  else
    printf '    -- some pods not yet healthy\n'
  fi
else
  printf '  Health Check:      pending\n'
fi

if [ -n "$METRICS_SCORE" ]; then
  printf '  Metrics:           %s' "$METRICS_SCORE"
  if [ "$METRICS_SCORE" = "0" ]; then
    printf '    -- no measurable improvement in available metrics\n'
    printf '                              (workload has no app-level Prometheus\n'
    printf '                               instrumentation; only cAdvisor data)\n'
  else
    printf '    -- average improvement across Prometheus queries\n'
  fi
else
  printf '  Metrics:           pending\n'
fi
printf '\n'
