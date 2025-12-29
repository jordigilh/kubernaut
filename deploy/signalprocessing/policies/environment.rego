# Environment Classification Policy
# Per IMPLEMENTATION_PLAN_V1.22.md Day 4 specification
#
# BR-SP-051: Primary detection from namespace labels (kubernaut.ai/environment)
# BR-SP-051: Case-insensitive matching (uses lower() function)
# BR-SP-052: DEPRECATED (2025-12-20) - ConfigMap fallback removed from Go code
#            Operators can implement ConfigMap-like logic in Rego if needed
# BR-SP-053: DEPRECATED (2025-12-20) - Go "unknown" default removed
#            Operators define their own defaults using `default` keyword in Rego
#
# IMPORTANT: This policy is MANDATORY. Go code has NO fallback logic.
# Operators MUST customize this policy to match their environment detection needs.
# The `default` rule at the bottom catches any unmatched combinations.
#
# Input Schema:
# {
#   "namespace": { "labels": { "kubernaut.ai/environment": "production" } },
#   "signal": { "labels": {...}, "annotations": {...} }
# }
#
# Output Schema:
# {
#   "environment": "production",
#   "source": "namespace-labels"
# }

package signalprocessing.environment

import rego.v1

# ============================================================================
# PRIMARY: Namespace Labels (kubernaut.ai/environment)
# ============================================================================
# Per BR-SP-051: Only kubernaut.ai/ prefixed labels
# Per BR-SP-051: Case-insensitive matching (normalize to lowercase)

result := {"environment": lower(env), "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# ============================================================================
# ADDITIONAL RULES (Operator Customization)
# ============================================================================
# Operators can add additional detection rules here.
# Examples:
#
# Detect from namespace name pattern:
# result := {"environment": "production", "source": "namespace-pattern"} if {
#     startswith(input.namespace.name, "prod-")
# }
#
# Detect from other labels:
# result := {"environment": lower(env), "source": "env-label"} if {
#     env := input.namespace.labels["env"]
#     env != ""
# }

# ============================================================================
# DEFAULT CATCH-ALL (MANDATORY)
# ============================================================================
# This is the AUTHORITATIVE default when no specific rule matches.
# Operators SHOULD customize this to match their environment strategy.
# Go code has NO fallback - this default is the single source of truth.
#
# Example customizations:
# - default result := {"environment": "development", "source": "default"} (safe default)
# - default result := {"environment": "", "source": "unclassified"} (empty = no classification)
#
default result := {"environment": "", "source": "unclassified"}
