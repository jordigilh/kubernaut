# Priority Assignment Policy for Gateway Service
#
# This Rego policy determines alert priority based on:
# - Severity (critical, warning, info)
# - Environment (production, staging, development)
# - Optional: Custom labels (team, service, etc.)
#
# Priority levels:
# - P0: Critical production issues (immediate response)
# - P1: High priority issues (response within 1 hour)
# - P2: Normal priority issues (best effort)
# - P3: Low priority issues (background processing)

package kubernaut.gateway.priority

import rego.v1

# Default priority if no rules match
default priority := "P2"

# P0: Critical production issues
priority := "P0" if {
    input.severity == "critical"
    input.environment == "production"
}

# P0: Critical issues for high-value teams (custom rule example)
priority := "P0" if {
    input.severity == "critical"
    input.labels["team"] == "platform-engineering"
}

# P1: Critical staging issues (pre-production testing)
priority := "P1" if {
    input.severity == "critical"
    input.environment == "staging"
}

# P1: Warning production issues (may escalate)
priority := "P1" if {
    input.severity == "warning"
    input.environment == "production"
}

# P2: Critical development issues
priority := "P2" if {
    input.severity == "critical"
    input.environment == "development"
}

# P2: Warning staging issues
priority := "P2" if {
    input.severity == "warning"
    input.environment == "staging"
}

# P2: Warning development issues
priority := "P2" if {
    input.severity == "warning"
    input.environment == "development"
}

# P3: Info alerts (any environment)
priority := "P3" if {
    input.severity == "info"
}

# P3: Unknown severity (safety default)
priority := "P3" if {
    not input.severity
}


