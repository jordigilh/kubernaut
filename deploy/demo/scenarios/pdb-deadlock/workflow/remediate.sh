#!/bin/sh
set -e

echo "=== Phase 1: Validate ==="
echo "Checking PDB $TARGET_PDB status..."

MIN_AVAILABLE=$(kubectl get pdb "$TARGET_PDB" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.minAvailable}')
ALLOWED=$(kubectl get pdb "$TARGET_PDB" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.disruptionsAllowed}')
CURRENT_HEALTHY=$(kubectl get pdb "$TARGET_PDB" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.currentHealthy}')
EXPECTED=$(kubectl get pdb "$TARGET_PDB" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.expectedPods}')
echo "Current minAvailable: $MIN_AVAILABLE"
echo "Disruptions allowed: $ALLOWED"
echo "Healthy/Expected: $CURRENT_HEALTHY/$EXPECTED"

if [ "$ALLOWED" -gt 0 ]; then
  echo "WARNING: PDB already allows $ALLOWED disruptions -- may have resolved itself."
  echo "Proceeding with relaxation as the alert indicated a deadlock."
fi

NEW_MIN=$((MIN_AVAILABLE - 1))
if [ "$NEW_MIN" -lt 1 ]; then
  NEW_MIN=1
fi
echo "Validated: will reduce minAvailable from $MIN_AVAILABLE to $NEW_MIN"

echo "=== Phase 2: Action ==="
echo "Patching PDB $TARGET_PDB minAvailable to $NEW_MIN..."
kubectl patch pdb "$TARGET_PDB" -n "$TARGET_NAMESPACE" \
  --type=merge -p "{\"spec\":{\"minAvailable\":$NEW_MIN}}"

echo "Waiting for disruption budget to update (10s)..."
sleep 10

echo "=== Phase 3: Verify ==="
NEW_MIN_AVAILABLE=$(kubectl get pdb "$TARGET_PDB" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.spec.minAvailable}')
NEW_ALLOWED=$(kubectl get pdb "$TARGET_PDB" -n "$TARGET_NAMESPACE" \
  -o jsonpath='{.status.disruptionsAllowed}')
echo "New minAvailable: $NEW_MIN_AVAILABLE"
echo "New disruptions allowed: $NEW_ALLOWED"

if [ "$NEW_ALLOWED" -gt 0 ]; then
  echo "=== SUCCESS: PDB relaxed ($MIN_AVAILABLE -> $NEW_MIN_AVAILABLE), disruptions now allowed: $NEW_ALLOWED ==="
else
  echo "WARNING: PDB still shows 0 allowed disruptions after patch -- may need more time to reconcile"
fi
