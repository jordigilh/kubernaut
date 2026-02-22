#!/usr/bin/env bash
# Display the EffectivenessAssessment result for the demo recording.
set -euo pipefail

NAMESPACE="${1:-demo-crashloop}"

EA_NAME=$(kubectl get effectivenessassessments -n "$NAMESPACE" -o jsonpath='{.items[0].metadata.name}')

PHASE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.phase}')
REASON=$(kubectl get effectivenessassessments "$EA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.assessmentReason}')
MESSAGE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.message}')
ALERT_SCORE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.components.alertScore}')
HEALTH_SCORE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.components.healthScore}')
METRICS_SCORE=$(kubectl get effectivenessassessments "$EA_NAME" -n "$NAMESPACE" -o jsonpath='{.status.components.metricsScore}')

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

# Alert Resolution: 1.0 means the alert cleared from AlertManager, 0.0 still firing
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

# Health Check: ratio of ready replicas to desired (1.0 = all healthy)
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

# Metrics: pre/post Prometheus comparison (average improvement across available queries)
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
