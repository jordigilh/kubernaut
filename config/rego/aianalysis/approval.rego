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

# Check if stateful workload
is_stateful if {
    input.detected_labels["stateful"] == true
}

# Check if target is validated
target_validated if {
    input.target_in_owner_chain == true
}

# Check if warnings exist
has_warnings if {
    count(input.warnings) > 0
}

# Check if failed detections exist
has_failed_detections if {
    count(input.failed_detections) > 0
}

# Non-production environments
is_production if {
    input.environment == "production"
}

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

# Recovery attempt detection
is_recovery_attempt if {
    input.is_recovery_attempt == true
}

# Multiple recovery attempts (3+) = higher risk
is_multiple_recovery if {
    is_recovery_attempt
    input.recovery_attempt_number >= 3
}

# ========================================
# APPROVAL RULES (Prioritized)
# ========================================

# Multiple recovery attempts require approval (any environment)
require_approval if {
    is_multiple_recovery
}

# Production + unvalidated target requires approval (BR-AI-013)
require_approval if {
    is_production
    not target_validated
}

# Production + failed detections requires approval (BR-AI-013)
require_approval if {
    is_production
    has_failed_detections
}

# Production + warnings requires approval (BR-AI-011)
require_approval if {
    is_production
    has_warnings
}

# Production + stateful workload requires approval (BR-AI-013)
require_approval if {
    is_production
    is_stateful
}

# Production environment requires approval (catch-all for production)
require_approval if {
    is_production
}

# ========================================
# REASON GENERATION (Prioritized - first match wins)
# ========================================

# Priority 1: Multiple recovery attempts (most critical)
reason := sprintf("Multiple recovery attempts (%d) - human approval required", [input.recovery_attempt_number]) if {
    require_approval
    is_multiple_recovery
}

# Priority 2: Production + unvalidated target
reason := "Production environment with unvalidated target - requires manual approval" if {
    require_approval
    is_production
    not is_multiple_recovery
    not target_validated
}

# Priority 3: Production + failed detections
reason := "Production environment with failed detections - requires manual approval" if {
    require_approval
    is_production
    not is_multiple_recovery
    target_validated
    has_failed_detections
}

# Priority 4: Production + warnings
reason := "Production environment with warnings - requires manual approval" if {
    require_approval
    is_production
    not is_multiple_recovery
    target_validated
    not has_failed_detections
    has_warnings
}

# Priority 5: Production + stateful workload
reason := "Production environment with Stateful workload - requires manual approval" if {
    require_approval
    is_production
    not is_multiple_recovery
    target_validated
    not has_failed_detections
    not has_warnings
    is_stateful
}

# Priority 6: Production (general)
reason := "Production environment requires manual approval" if {
    require_approval
    is_production
    not is_multiple_recovery
    target_validated
    not has_failed_detections
    not has_warnings
    not is_stateful
}



