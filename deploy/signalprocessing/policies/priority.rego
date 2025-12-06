# Priority Assignment Policy
# Per IMPLEMENTATION_PLAN_V1.22.md Day 5 specification
#
# BR-SP-070: Priority assignment via Rego policy
# BR-SP-071: Severity-based fallback handled in Go code
#
# Input Schema (per BR-SP-070):
# {
#   "signal": { "severity": "critical", "source": "prometheus" },
#   "environment": "production",
#   "namespace_labels": { "tier": "critical" },
#   "deployment_labels": { "app": "payment-service" }
# }
#
# Output Schema:
# {
#   "priority": "P0",
#   "confidence": 0.95,
#   "policy_name": "critical-production"
# }
#
# Priority Levels (P0-P3 only per BR-SP-071):
# - P0: Critical production issues requiring immediate attention
# - P1: High priority issues (critical non-prod or warning production)
# - P2: Medium priority (info production or warning non-prod)
# - P3: Low priority (development/test or info non-prod)

package signalprocessing.priority

import rego.v1

# ============================================================================
# P0: Critical Production
# ============================================================================

# P0: Critical severity + production environment
result := {"priority": "P0", "confidence": 0.95, "policy_name": "critical-production"} if {
    lower(input.signal.severity) == "critical"
    lower(input.environment) == "production"
}

# P0: Critical severity + tier=critical namespace label
result := {"priority": "P0", "confidence": 0.92, "policy_name": "critical-tier-label"} if {
    lower(input.signal.severity) == "critical"
    input.namespace_labels["tier"] == "critical"
}

# ============================================================================
# P1: High Priority
# ============================================================================

# P1: Critical severity + staging environment
result := {"priority": "P1", "confidence": 0.90, "policy_name": "critical-staging"} if {
    lower(input.signal.severity) == "critical"
    lower(input.environment) == "staging"
}

# P1: Warning severity + production environment
result := {"priority": "P1", "confidence": 0.90, "policy_name": "warning-production"} if {
    lower(input.signal.severity) == "warning"
    lower(input.environment) == "production"
}

# P1: Critical severity + tier=high namespace label
result := {"priority": "P1", "confidence": 0.88, "policy_name": "critical-tier-high"} if {
    lower(input.signal.severity) == "critical"
    input.namespace_labels["tier"] == "high"
}

# ============================================================================
# P2: Medium Priority
# ============================================================================

# P2: Warning severity + staging environment
result := {"priority": "P2", "confidence": 0.85, "policy_name": "warning-staging"} if {
    lower(input.signal.severity) == "warning"
    lower(input.environment) == "staging"
}

# P2: Info severity + production environment
result := {"priority": "P2", "confidence": 0.85, "policy_name": "info-production"} if {
    lower(input.signal.severity) == "info"
    lower(input.environment) == "production"
}

# P2: Critical severity + development environment
result := {"priority": "P2", "confidence": 0.85, "policy_name": "critical-development"} if {
    lower(input.signal.severity) == "critical"
    lower(input.environment) == "development"
}

# P2: Critical severity + test environment
result := {"priority": "P2", "confidence": 0.85, "policy_name": "critical-test"} if {
    lower(input.signal.severity) == "critical"
    lower(input.environment) == "test"
}

# ============================================================================
# P3: Low Priority
# ============================================================================

# P3: Warning severity + development environment
result := {"priority": "P3", "confidence": 0.80, "policy_name": "warning-development"} if {
    lower(input.signal.severity) == "warning"
    lower(input.environment) == "development"
}

# P3: Warning severity + test environment
result := {"priority": "P3", "confidence": 0.80, "policy_name": "warning-test"} if {
    lower(input.signal.severity) == "warning"
    lower(input.environment) == "test"
}

# P3: Info severity + staging environment
result := {"priority": "P3", "confidence": 0.80, "policy_name": "info-staging"} if {
    lower(input.signal.severity) == "info"
    lower(input.environment) == "staging"
}

# P3: Info severity + development environment
result := {"priority": "P3", "confidence": 0.80, "policy_name": "info-development"} if {
    lower(input.signal.severity) == "info"
    lower(input.environment) == "development"
}

# P3: Info severity + test environment
result := {"priority": "P3", "confidence": 0.80, "policy_name": "info-test"} if {
    lower(input.signal.severity) == "info"
    lower(input.environment) == "test"
}

# ============================================================================
# Default Fallback (handled by Go code per BR-SP-071)
# This rule catches any unmatched combinations
# ============================================================================

# Default: Return P2 with low confidence when no specific rule matches
# This enables Rego to always return a result
# The Go code's fallbackBySeverity() provides the authoritative fallback
result := {"priority": "P2", "confidence": 0.60, "policy_name": "default-fallback"} if {
    not matched_specific_rule
}

# Helper to check if any specific rule matched
matched_specific_rule if {
    lower(input.signal.severity) == "critical"
}

matched_specific_rule if {
    lower(input.signal.severity) == "warning"
}

matched_specific_rule if {
    lower(input.signal.severity) == "info"
}

