# AI Analysis Approval Policy
# DD-WORKFLOW-001 v2.2: Approval determination based on environment and data quality
# Business Requirements: BR-AI-011 (policy evaluation), BR-AI-013 (approval scenarios), BR-AI-014 (graceful degradation)

package aianalysis.approval

import rego.v1

# ========================================
# DEFAULT VALUES
# ========================================

default require_approval := false
default reason := "Auto-approved"

# ========================================
# PRODUCTION ENVIRONMENT RULES
# ========================================

# Production environment with unvalidated target requires approval
require_approval if {
    input.environment == "production"
    not input.target_in_owner_chain
}

reason := "Production environment with unvalidated target requires manual approval" if {
    input.environment == "production"
    not input.target_in_owner_chain
}

# Production with failed detections requires approval (DD-WORKFLOW-001 v2.2)
require_approval if {
    input.environment == "production"
    count(input.failed_detections) > 0
}

reason := concat("", ["Production environment with failed detections: ", concat(", ", input.failed_detections)]) if {
    input.environment == "production"
    count(input.failed_detections) > 0
}

# Production with warnings requires approval
require_approval if {
    input.environment == "production"
    count(input.warnings) > 0
}

reason := "Production environment with warnings requires manual approval" if {
    input.environment == "production"
    count(input.warnings) > 0
}

# ========================================
# HELPER FUNCTIONS
# ========================================

# Check if a specific detection failed
detection_failed(field) if {
    field in input.failed_detections
}

# Check if any critical detection failed
has_critical_detection_failure if {
    detection_failed("gitOpsManaged")
}

has_critical_detection_failure if {
    detection_failed("pdbProtected")
}

# ========================================
# CRITICAL WORKLOAD PROTECTION RULES
# ========================================

# Stateful workloads in production require approval
require_approval if {
    input.environment == "production"
    input.detected_labels.stateful == true
}

reason := "Stateful workload in production requires manual approval" if {
    input.environment == "production"
    input.detected_labels.stateful == true
}

# ========================================
# AUTO-APPROVE RULES
# ========================================

# Non-production environments auto-approve (unless overridden by specific rules)
# This is handled by the default values - if no require_approval rule matches,
# the default (false) applies

# Development environment explicitly auto-approved
not_production if {
    input.environment == "development"
}

not_production if {
    input.environment == "staging"
}

not_production if {
    input.environment == "qa"
}

not_production if {
    input.environment == "test"
}


