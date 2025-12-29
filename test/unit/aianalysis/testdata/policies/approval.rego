# AIAnalysis Approval Policy - Test Version (OPA v1 syntax)
# BR-AI-013: Determines if human approval is required for remediation
# Per IMPLEMENTATION_PLAN_V1.0.md lines 1834-1888

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

# =============================================================================
# Approval Rules
# =============================================================================

# Default: require approval for production, auto-approve for non-production
default require_approval := false

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
# Reason Generation (Prioritized - first match wins)
# =============================================================================

# Priority 1: Multiple recovery attempts (most critical)
reason := msg if {
    require_approval
    is_multiple_recovery
    msg := sprintf("Multiple recovery attempts (%d) - human approval required", [input.recovery_attempt_number])
}

# Priority 2: High severity + recovery
reason := msg if {
    require_approval
    is_high_severity
    is_recovery_attempt
    not is_multiple_recovery  # Only if not already matched by priority 1
    msg := "High severity + recovery attempt - human approval required"
}

# Priority 3: Production environment
reason := msg if {
    require_approval
    is_production
    not is_multiple_recovery
    not is_high_severity
    msg := "Production environment requires manual approval"
}

# Auto-approve case
reason := "Auto-approved by policy" if {
    not require_approval
}
