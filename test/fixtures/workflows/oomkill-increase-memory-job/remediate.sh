#!/bin/sh
# OOMKill Remediation Script: Increase Memory Limits
#
# Authority: DD-WORKFLOW-003 (Parameterized Remediation Actions)
# Authority: ADR-043 (Workflow Schema Definition Standard)
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
#
# Pattern: Validate -> Action -> Verify (DD-WORKFLOW-003)
#
# Parameters (env vars, UPPER_SNAKE_CASE per DD-WORKFLOW-003):
#   TARGET_RESOURCE_KIND  - Kubernetes resource kind (Deployment, StatefulSet, DaemonSet)
#   TARGET_RESOURCE_NAME  - Name of the resource to patch
#   TARGET_NAMESPACE      - Namespace of the resource
#   MEMORY_LIMIT_NEW      - New memory limit to apply (e.g., 256Mi, 1Gi)
#
# Fallback (injected by WE Job executor):
#   TARGET_RESOURCE       - Combined format: namespace/kind/name
#
set -e

echo "============================================"
echo "OOMKill Remediation: Increase Memory Limits"
echo "============================================"
echo ""

# ============================================
# Parameter Resolution
# Prefer individual parameters from LLM over combined TARGET_RESOURCE
# ============================================
if [ -n "$TARGET_RESOURCE_KIND" ] && [ -n "$TARGET_RESOURCE_NAME" ] && [ -n "$TARGET_NAMESPACE" ]; then
    KIND="$TARGET_RESOURCE_KIND"
    NAME="$TARGET_RESOURCE_NAME"
    NAMESPACE="$TARGET_NAMESPACE"
    echo "Using LLM-provided parameters:"
elif [ -n "$TARGET_RESOURCE" ]; then
    # Fallback: parse TARGET_RESOURCE (format: namespace/kind/name)
    NAMESPACE=$(echo "$TARGET_RESOURCE" | cut -d'/' -f1)
    KIND=$(echo "$TARGET_RESOURCE" | cut -d'/' -f2)
    NAME=$(echo "$TARGET_RESOURCE" | cut -d'/' -f3)
    echo "Using TARGET_RESOURCE fallback:"
else
    echo "ERROR: No target resource specified."
    echo "  Required: TARGET_RESOURCE_KIND + TARGET_RESOURCE_NAME + TARGET_NAMESPACE"
    echo "  Or: TARGET_RESOURCE (format: namespace/kind/name)"
    exit 1
fi

# Normalize kind to lowercase for kubectl
KIND_LOWER=$(echo "$KIND" | tr '[:upper:]' '[:lower:]')

echo "  Kind:      $KIND ($KIND_LOWER)"
echo "  Name:      $NAME"
echo "  Namespace: $NAMESPACE"
echo "  New Limit: $MEMORY_LIMIT_NEW"
echo ""

# Validate MEMORY_LIMIT_NEW is set
if [ -z "$MEMORY_LIMIT_NEW" ]; then
    echo "ERROR: MEMORY_LIMIT_NEW is not set."
    exit 1
fi

# ============================================
# Phase 1: Validate
# ============================================
echo "--- Phase 1: Validate ---"
echo "Checking that $KIND_LOWER/$NAME exists in namespace $NAMESPACE..."

if ! kubectl get "$KIND_LOWER" "$NAME" -n "$NAMESPACE" > /dev/null 2>&1; then
    echo "ERROR: $KIND_LOWER/$NAME not found in namespace $NAMESPACE"
    exit 1
fi

CURRENT_LIMIT=$(kubectl get "$KIND_LOWER" "$NAME" -n "$NAMESPACE" \
    -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}' 2>/dev/null || echo "not set")
echo "Current memory limit: $CURRENT_LIMIT"
echo "Target memory limit:  $MEMORY_LIMIT_NEW"
echo ""

# ============================================
# Phase 2: Action
# ============================================
echo "--- Phase 2: Action ---"
echo "Patching $KIND_LOWER/$NAME memory limit to $MEMORY_LIMIT_NEW..."

kubectl patch "$KIND_LOWER" "$NAME" -n "$NAMESPACE" --type='json' \
    -p="[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/0/resources/limits/memory\",\"value\":\"$MEMORY_LIMIT_NEW\"}]"

echo "Patch applied successfully."
echo ""

# ============================================
# Phase 3: Verify
# ============================================
echo "--- Phase 3: Verify ---"
echo "Verifying memory limit was updated..."

NEW_LIMIT=$(kubectl get "$KIND_LOWER" "$NAME" -n "$NAMESPACE" \
    -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')
echo "Verified memory limit: $NEW_LIMIT"

if [ "$NEW_LIMIT" = "$MEMORY_LIMIT_NEW" ]; then
    echo ""
    echo "============================================"
    echo "SUCCESS: Memory limit updated to $MEMORY_LIMIT_NEW"
    echo "============================================"
    exit 0
else
    echo ""
    echo "ERROR: Verification failed!"
    echo "  Expected: $MEMORY_LIMIT_NEW"
    echo "  Got:      $NEW_LIMIT"
    exit 1
fi
