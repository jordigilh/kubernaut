# Environment Classification Policy
# Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification
#
# BR-SP-051: Primary detection from namespace labels (kubernaut.ai/environment)
# BR-SP-053: Default to "unknown" when all methods fail
#
# Confidence Levels:
# - 0.95: Namespace label (primary source, high confidence)
# - 0.0:  Default fallback (no detection possible)

package signalprocessing.environment

# Primary: Namespace labels (kubernaut.ai/environment)
# Per BR-SP-051: Only kubernaut.ai/ prefixed labels
# Confidence: 0.95
result := {"environment": env, "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# Default fallback
# Per BR-SP-053: Return "unknown" when detection fails
# Confidence: 0.0
result := {"environment": "unknown", "confidence": 0.0, "source": "default"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
}

