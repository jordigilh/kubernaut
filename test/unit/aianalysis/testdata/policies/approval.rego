# AIAnalysis Approval Policy - Test Version (OPA v1 syntax)
# BR-AI-013: Determines if human approval is required for remediation

package aianalysis.approval

# Non-production environments are always auto-approved
# BR-AI-013: Only production requires approval checks
is_production if {
    input.environment == "production"
}

# Default: require approval for production, auto-approve for non-production
default require_approval := false

# Only production environment requires approval checks
require_approval if {
    is_production
    not input.target_in_owner_chain
}

require_approval if {
    is_production
    count(input.failed_detections) > 0
}

require_approval if {
    is_production
    count(input.warnings) > 0
}

require_approval if {
    is_production
    input.confidence < 0.7
}

# Generate reason string
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

reason := "Auto-approved by policy" if {
    not require_approval
}
