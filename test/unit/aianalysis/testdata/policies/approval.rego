# AIAnalysis Approval Policy - Test Version (OPA v1 syntax)
# BR-AI-013: Determines if human approval is required for remediation
# Per IMPLEMENTATION_PLAN_V1.0.md lines 1834-1888

package aianalysis.approval

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

# =============================================================================
# Approval Rules
# =============================================================================

# Default: require approval for production, auto-approve for non-production
default require_approval := false

# Production + target not in owner chain = approval required
require_approval if {
    is_production
    not input.target_in_owner_chain
}

# Production + failed detections = approval required
require_approval if {
    is_production
    count(input.failed_detections) > 0
}

# Production + warnings = approval required
require_approval if {
    is_production
    count(input.warnings) > 0
}

# Production + low confidence = approval required
require_approval if {
    is_production
    input.confidence < 0.7
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
# Reason Generation
# =============================================================================

reason := msg if {
    require_approval
    not input.target_in_owner_chain
    msg := "Target resource not found in owner chain - manual verification required"
}

reason := msg if {
    require_approval
    count(input.failed_detections) > 0
    msg := sprintf("Failed label detections: %v", [input.failed_detections])
}

reason := msg if {
    require_approval
    count(input.warnings) > 0
    msg := sprintf("Warnings present: %v", [input.warnings])
}

reason := msg if {
    require_approval
    input.confidence < 0.7
    msg := sprintf("Low confidence score: %.2f - human review recommended", [input.confidence])
}

reason := msg if {
    require_approval
    is_multiple_recovery
    msg := sprintf("Multiple recovery attempts (%d) - human approval required", [input.recovery_attempt_number])
}

reason := msg if {
    require_approval
    is_high_severity
    is_recovery_attempt
    msg := "High severity + recovery attempt - human approval required"
}

reason := "Auto-approved by policy" if {
    not require_approval
}
