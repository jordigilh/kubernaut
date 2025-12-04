# AI Analysis Approval Policy
# DD-WORKFLOW-001 v2.2: Approval determination based on environment and data quality

package aianalysis.approval

import rego.v1

default require_approval := false
default reason := "Auto-approved"

# Production environment with data quality issues requires approval
require_approval if {
    input.environment == "production"
    not input.target_in_owner_chain
}

reason := "Production environment with unvalidated target requires manual approval" if {
    input.environment == "production"
    not input.target_in_owner_chain
}

# Production with failed detections requires approval
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

