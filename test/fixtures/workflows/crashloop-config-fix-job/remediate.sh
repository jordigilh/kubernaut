#!/bin/sh
# CrashLoop Config Fix Remediation Script: Patch ConfigMap + Restart Deployment
#
# Authority: DD-WORKFLOW-003 (Parameterized Remediation Actions)
# Authority: ADR-043 (Workflow Schema Definition Standard)
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
# Issue: #1542 (real, verifiable fix — not a no-op simulation)
#
# Pattern: Validate -> Action -> Verify (DD-WORKFLOW-003)
#
# Fixes a CrashLoopBackOff caused by an invalid ConfigMap value by patching
# the offending key back to a known-good value and restarting the Deployment
# so pods pick up the corrected configuration. Verifies the rollout completes
# and no pod remains in CrashLoopBackOff.
#
# Target resource resolution (mirrors oomkill-increase-memory-job/remediate.sh):
#   TARGET_RESOURCE_KIND + TARGET_RESOURCE_NAME + TARGET_RESOURCE_NAMESPACE
#     - Preferred, when the LLM explicitly selects these as parameters.
#   TARGET_RESOURCE (format: namespace/kind/name)
#     - Fallback, always injected by the WE Job executor from the real
#       RemediationRequest target resource (BR-WE-014).
#
# Required parameters (env vars, UPPER_SNAKE_CASE per DD-WORKFLOW-003):
#   CONFIGMAP_NAME  - Name of the ConfigMap containing the offending key
#   CONFIGMAP_KEY   - ConfigMap key to correct
#   CONFIGMAP_VALUE - Known-good value to set for CONFIGMAP_KEY
set -e

echo "============================================"
echo "CrashLoop Config Fix - Remediation Job"
echo "============================================"
echo ""

# ============================================
# Target Resource Resolution
# Prefer individual parameters from LLM over combined TARGET_RESOURCE
# ============================================
if [ -n "$TARGET_RESOURCE_KIND" ] && [ -n "$TARGET_RESOURCE_NAME" ] && [ -n "$TARGET_RESOURCE_NAMESPACE" ]; then
    KIND="$TARGET_RESOURCE_KIND"
    DEPLOYMENT_NAME="$TARGET_RESOURCE_NAME"
    NAMESPACE="$TARGET_RESOURCE_NAMESPACE"
    echo "Using LLM-provided target resource parameters:"
elif [ -n "$TARGET_RESOURCE" ]; then
    # Fallback: parse TARGET_RESOURCE (format: namespace/kind/name)
    NAMESPACE=$(echo "$TARGET_RESOURCE" | cut -d'/' -f1)
    KIND=$(echo "$TARGET_RESOURCE" | cut -d'/' -f2)
    DEPLOYMENT_NAME=$(echo "$TARGET_RESOURCE" | cut -d'/' -f3)
    echo "Using TARGET_RESOURCE fallback:"
else
    echo "ERROR: No target resource specified."
    echo "  Required: TARGET_RESOURCE_KIND + TARGET_RESOURCE_NAME + TARGET_RESOURCE_NAMESPACE"
    echo "  Or: TARGET_RESOURCE (format: namespace/kind/name)"
    exit 1
fi

# ============================================
# Parameter Validation
# CONFIGMAP_* must be injected by the WE Job executor from the LLM-selected
# workflow parameters.
# ============================================
MISSING=""

[ -z "$CONFIGMAP_NAME" ] && MISSING="${MISSING}  - CONFIGMAP_NAME\n"
[ -z "$CONFIGMAP_KEY" ] && MISSING="${MISSING}  - CONFIGMAP_KEY\n"
[ -z "$CONFIGMAP_VALUE" ] && MISSING="${MISSING}  - CONFIGMAP_VALUE\n"

if [ -n "$MISSING" ]; then
    echo "ERROR: Required parameters are missing:"
    printf "%b" "$MISSING"
    echo ""
    echo "These parameters must be provided by the LLM via the"
    echo "workflow selection response and propagated through the"
    echo "AIAnalysis -> WorkflowExecution -> Job pipeline."
    exit 1
fi

echo "  Kind:            $KIND"
echo "  Deployment:      $DEPLOYMENT_NAME"
echo "  Namespace:       $NAMESPACE"
echo "  ConfigMap:       $CONFIGMAP_NAME"
echo "  ConfigMap Key:   $CONFIGMAP_KEY"
echo "  ConfigMap Value: $CONFIGMAP_VALUE"
echo ""

# ============================================
# Phase 1: Validate
# ============================================
echo "--- Phase 1: Validate ---"

if ! kubectl get deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" > /dev/null 2>&1; then
    echo "ERROR: deployment/$DEPLOYMENT_NAME not found in namespace $NAMESPACE"
    exit 1
fi
echo "✅ deployment/$DEPLOYMENT_NAME found"

if ! kubectl get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" > /dev/null 2>&1; then
    echo "ERROR: configmap/$CONFIGMAP_NAME not found in namespace $NAMESPACE"
    exit 1
fi
echo "✅ configmap/$CONFIGMAP_NAME found"

CURRENT_VALUE=$(kubectl get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" \
    -o jsonpath="{.data.$CONFIGMAP_KEY}" 2>/dev/null || echo "")
echo "Current $CONFIGMAP_KEY: '$CURRENT_VALUE'"
echo "Target  $CONFIGMAP_KEY: '$CONFIGMAP_VALUE'"
echo ""

# ============================================
# Phase 2: Action
# ============================================
echo "--- Phase 2: Action ---"
echo "Patching configmap/$CONFIGMAP_NAME key $CONFIGMAP_KEY -> $CONFIGMAP_VALUE..."

kubectl patch configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" --type merge \
    -p="{\"data\":{\"$CONFIGMAP_KEY\":\"$CONFIGMAP_VALUE\"}}"
echo "✅ ConfigMap patched"

echo "Restarting deployment/$DEPLOYMENT_NAME to pick up corrected configuration..."
kubectl rollout restart deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE"
echo "✅ Rollout restart triggered"
echo ""

# ============================================
# Phase 3: Verify
# ============================================
echo "--- Phase 3: Verify ---"
echo "Verifying ConfigMap reflects the corrected value..."

NEW_VALUE=$(kubectl get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" \
    -o jsonpath="{.data.$CONFIGMAP_KEY}")
if [ "$NEW_VALUE" != "$CONFIGMAP_VALUE" ]; then
    echo ""
    echo "ERROR: Verification failed!"
    echo "  Expected $CONFIGMAP_KEY: $CONFIGMAP_VALUE"
    echo "  Got:      $NEW_VALUE"
    exit 1
fi
echo "✅ ConfigMap verified: $CONFIGMAP_KEY=$NEW_VALUE"

echo "Waiting for deployment rollout to complete (timeout: 120s)..."
if ! kubectl rollout status deployment "$DEPLOYMENT_NAME" -n "$NAMESPACE" --timeout=120s; then
    echo ""
    echo "ERROR: Deployment rollout did not complete successfully"
    kubectl get pods -n "$NAMESPACE" -l "app=$DEPLOYMENT_NAME" 2>/dev/null || true
    exit 1
fi
echo "✅ Deployment rollout completed"

echo "Verifying no pod remains in CrashLoopBackOff..."
BAD_PODS=$(kubectl get pods -n "$NAMESPACE" -l "app=$DEPLOYMENT_NAME" \
    -o jsonpath='{range .items[*]}{.status.containerStatuses[*].state.waiting.reason}{"\n"}{end}' 2>/dev/null \
    | grep -c "CrashLoopBackOff" || true)
if [ "$BAD_PODS" -gt 0 ] 2>/dev/null; then
    echo ""
    echo "ERROR: Verification failed! $BAD_PODS pod(s) still in CrashLoopBackOff"
    kubectl get pods -n "$NAMESPACE" -l "app=$DEPLOYMENT_NAME"
    exit 1
fi

echo ""
echo "============================================"
echo "SUCCESS: Remediation completed — $CONFIGMAP_KEY corrected, deployment healthy"
echo "============================================"
