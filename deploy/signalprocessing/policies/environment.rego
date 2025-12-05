# Environment Classification Policy
# Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification
#
# BR-SP-051: Primary detection from namespace labels (kubernaut.ai/environment)
# BR-SP-052: ConfigMap fallback handled in Go code
# BR-SP-053: Default to "unknown" when all methods fail
#
# Priority Order (per plan):
# 1. Namespace labels (confidence: 0.95)
# 2. Signal labels (confidence: 0.80)
# 3. Default (confidence: 0.0)
#
# Confidence Levels:
# - 0.95: Namespace label (primary source, high confidence)
# - 0.80: Signal label (secondary source, medium confidence)
# - 0.0:  Default fallback (no detection possible)

package signalprocessing.environment

# Primary: Namespace labels (kubernaut.ai/environment)
# Per BR-SP-051: Only kubernaut.ai/ prefixed labels
# Confidence: 0.95
result := {"environment": env, "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# Secondary: Signal labels (kubernaut.ai/environment)
# Per plan specification: When namespace label absent, try signal labels
# Confidence: 0.80
result := {"environment": env, "confidence": 0.80, "source": "signal-labels"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    env := input.signal.labels["kubernaut.ai/environment"]
    env != ""
}

# Default fallback
# Per BR-SP-053: Return "unknown" when detection fails
# Confidence: 0.0
result := {"environment": "unknown", "confidence": 0.0, "source": "default"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    not input.signal.labels["kubernaut.ai/environment"]
}
