# AIAnalysis Approval Policy - Test Version (scored risk factors)
# BR-AI-013: Determines if human approval is required for remediation
# Issue #98: Refactored from exclusion chains to scored risk factors

package aianalysis.approval

import rego.v1

# =============================================================================
# Helper Rules
# =============================================================================

# Non-production environments are always auto-approved
# BR-AI-013: Only production requires approval checks
is_production if {
    input.environment == "production"
}

# Recovery attempt detection
is_recovery_attempt if {
    input.is_recovery_attempt == true
}

# Multiple recovery attempts (3+) = higher risk
is_multiple_recovery if {
    is_recovery_attempt
    input.recovery_attempt_number >= 3
}

# High severity signal
is_high_severity if {
    input.severity == "critical"
}

is_high_severity if {
    input.severity == "P0"
}

# ADR-055: Check if affected_resource is present
has_affected_resource if {
    input.affected_resource
    input.affected_resource.kind != ""
}

# =============================================================================
# Approval Rules (independent boolean checks)
# =============================================================================

default require_approval := false

# BR-AI-085-005: Default-deny when affected_resource is missing (ADR-055)
require_approval if {
    not has_affected_resource
}

# Production environment ALWAYS requires approval (BR-AI-013)
require_approval if {
    is_production
}

# Multiple recovery attempts = approval required (any environment)
# BR-AI-013: Escalating approval for repeated failures
require_approval if {
    is_multiple_recovery
}

# High severity + recovery = approval required
require_approval if {
    is_high_severity
    is_recovery_attempt
}

# =============================================================================
# Scored Risk Factors for Reason Generation
# =============================================================================
# Each risk factor independently contributes a scored entry.
# The highest-scored reason wins. No exclusion chains needed.

risk_factors contains {"score": 90, "reason": "Missing affected resource - cannot determine remediation target (BR-AI-085-005)"} if {
    not has_affected_resource
}

risk_factors contains {"score": 100, "reason": msg} if {
    is_multiple_recovery
    msg := sprintf("Multiple recovery attempts (%d) - human approval required", [input.recovery_attempt_number])
}

risk_factors contains {"score": 80, "reason": "High severity + recovery attempt - human approval required"} if {
    is_high_severity
    is_recovery_attempt
}

risk_factors contains {"score": 40, "reason": "Production environment requires manual approval"} if {
    is_production
}

# =============================================================================
# Reason Aggregation: Highest score wins
# =============================================================================

all_scores contains f.score if {
    some f in risk_factors
}

max_risk_score := max(all_scores) if {
    count(all_scores) > 0
}

reason := f.reason if {
    some f in risk_factors
    f.score == max_risk_score
}

# Auto-approve case (no risk factors)
default reason := "Auto-approved by policy"
