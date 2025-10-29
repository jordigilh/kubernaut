# Kubernaut Gateway - Priority Assignment Policy
# BR-GATEWAY-013: Rego-based priority assignment
#
# This policy assigns priority levels (P0-P3) based on:
# - Alert severity (critical, warning, info)
# - Environment (production, staging, development)
# - Custom business rules

package kubernaut.gateway.priority

# Default priority for unknown cases
default priority = "P3"

# ============================================================================
# CUSTOM RULES (Evaluated first - more specific rules take precedence)
# ============================================================================

# Custom rule: Escalate database alerts in production to P0
priority = "P0" {
    input.severity == "warning"
    input.environment == "production"
    alert_lower := lower(input.alert_name)
    contains(alert_lower, "database")
}

# Custom rule: Escalate memory pressure in production to P0
# Only when BOTH memory alert AND critical threshold label present
priority = "P0" {
    input.severity == "warning"
    input.environment == "production"
    alert_lower := lower(input.alert_name)
    contains(alert_lower, "memory")
    # Check threshold label exists and equals "critical"
    threshold := input.labels.threshold
    threshold == "critical"
}

# ============================================================================
# STANDARD RULES (Base priority matrix)
# ============================================================================

# P0: Critical alerts in production (revenue-impacting outages)
priority = "P0" {
    input.severity == "critical"
    input.environment == "production"
}

# P1: Critical alerts in non-production OR warnings in production
priority = "P1" {
    input.severity == "critical"
    input.environment != "production"
}

priority = "P1" {
    input.severity == "warning"
    input.environment == "production"
    # Don't match if custom rules apply
    alert_lower := lower(input.alert_name)
    not contains(alert_lower, "database")
    # Exclude memory alerts only if they have critical threshold
    not memory_with_critical_threshold
}

# Helper rule: Check if this is a memory alert with critical threshold
memory_with_critical_threshold {
    alert_lower := lower(input.alert_name)
    contains(alert_lower, "memory")
    input.labels.threshold == "critical"
}

# P2: Warnings in non-production OR info in production
priority = "P2" {
    input.severity == "warning"
    input.environment != "production"
}

priority = "P2" {
    input.severity == "info"
    input.environment == "production"
}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}


}

# P3: Info in non-production (lowest priority)
priority = "P3" {
    input.severity == "info"
    input.environment != "production"
}

