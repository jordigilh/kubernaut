# Kubernaut Gateway - Remediation Path Policy
# BR-GATEWAY-014: Rego-based remediation path decision
#
# This policy determines remediation aggressiveness:
# - "ai-driven": Let AI analyze and suggest actions (safest)
# - "semi-automated": AI suggests, human approves
# - "automated": Execute approved actions automatically (most aggressive)

package kubernaut.gateway.remediation

# Default remediation path (safest)
default remediation_path = "ai-driven"

# Automated remediation for non-production environments
remediation_path = "automated" {
    input.environment != "production"
    input.priority != "P0"
}

# Semi-automated for production warnings
remediation_path = "semi-automated" {
    input.environment == "production"
    input.severity == "warning"
}

# AI-driven only for production critical alerts (safest)
remediation_path = "ai-driven" {
    input.environment == "production"
    input.severity == "critical"
}

# Custom rule: Always use AI-driven for database issues
remediation_path = "ai-driven" {
    contains(lower(input.alert_name), "database")
}

# Custom rule: Automated for known safe actions
remediation_path = "automated" {
    input.environment != "production"
    input.action_type == "restart_pod"
    input.labels["safe_to_restart"] == "true"
}

# Custom rule: Semi-automated for scaling operations
remediation_path = "semi-automated" {
    input.action_type == "scale_deployment"
}


