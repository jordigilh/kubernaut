# Environment Classification Policy
# Per IMPLEMENTATION_PLAN_V1.22.md Day 4 specification
#
# BR-SP-051: Primary detection from namespace labels (kubernaut.ai/environment)
# BR-SP-051: Case-insensitive matching (uses lower() function)
# BR-SP-052: ConfigMap fallback handled in Go code (between namespace and signal)
# BR-SP-053: Default to "unknown" when all methods fail
#
# Priority Order (per plan line 1864):
# 1. Namespace labels (confidence: 0.95) - Rego
# 2. ConfigMap (confidence: 0.75) - Go code
# 3. Signal labels (confidence: 0.80) - Go code (after ConfigMap)
# 4. Default (confidence: 0.0) - Go code
#
# Note: This Rego policy only handles namespace labels.
# ConfigMap and signal labels fallback are handled in Go code
# to maintain correct priority order per plan specification.

package signalprocessing.environment

# Primary: Namespace labels (kubernaut.ai/environment)
# Per BR-SP-051: Only kubernaut.ai/ prefixed labels
# Per BR-SP-051: Case-insensitive matching (normalize to lowercase)
# Confidence: 0.95
result := {"environment": lower(env), "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# Default fallback (when namespace label not present)
# Returns "unknown" so Go code can try ConfigMap and signal labels
result := {"environment": "unknown", "confidence": 0.0, "source": "default"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
}
