# Remediation Path Decision Policy for Gateway Service
#
# This Rego policy determines the remediation path based on:
# - Environment (production, staging, development)
# - Priority (P0, P1, P2, P3)
# - Optional: Alert type, resource type
#
# Remediation paths:
# - aggressive: Immediate automated execution (high confidence)
# - moderate: Automated with validation checks
# - conservative: GitOps PR for manual approval
# - manual: Analysis only, operator decides

package kubernaut.gateway.remediation

import rego.v1

# Default path if no rules match (safety first)
default path := "manual"

# Aggressive path: P0 production (immediate action)
path := "aggressive" if {
    input.priority == "P0"
    input.environment == "production"
}

# Aggressive path: P0/P1 development (fast feedback)
path := "aggressive" if {
    input.priority in ["P0", "P1"]
    input.environment == "development"
}

# Conservative path: P1 production (GitOps PR)
path := "conservative" if {
    input.priority == "P1"
    input.environment == "production"
}

# Moderate path: P0/P1 staging (automated validation)
path := "moderate" if {
    input.priority in ["P0", "P1"]
    input.environment == "staging"
}

# Moderate path: P2 development
path := "moderate" if {
    input.priority == "P2"
    input.environment == "development"
}

# Manual path: P2 production (human review)
path := "manual" if {
    input.priority == "P2"
    input.environment == "production"
}

# Manual path: P2 staging (human review)
path := "manual" if {
    input.priority == "P2"
    input.environment == "staging"
}

# Manual path: Unknown environment (safety first)
path := "manual" if {
    not input.environment in ["production", "staging", "development"]
}

# Manual path: Unknown priority (safety first)
path := "manual" if {
    not input.priority in ["P0", "P1", "P2", "P3"]
}


