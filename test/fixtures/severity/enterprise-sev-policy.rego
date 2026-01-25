# Enterprise Severity Scheme (Sev1-4) - Test Fixture
# DD-SEVERITY-001 v1.1: Custom severity mapping for testing
#
# This policy demonstrates Enterprise "Sev" severity scheme mapping
# to normalized severity values (critical, high, medium, low, unknown)
#
# Usage in tests:
#   - Load this policy into SignalProcessing classifier
#   - Send alerts with severity="Sev1", "Sev2", "Sev3", "Sev4"
#   - Verify normalized severity in SignalProcessing.Status.Severity

package signalprocessing.severity

import rego.v1

# Enterprise Sev1 → Critical (production outage)
result := {"severity": "critical", "source": "rego-policy"} if {
	input.signal.severity in ["Sev1", "SEV1", "sev1"]
}

# Enterprise Sev2 → High (degraded service)
result := {"severity": "high", "source": "rego-policy"} if {
	input.signal.severity in ["Sev2", "SEV2", "sev2"]
}

# Enterprise Sev3 → Medium (non-critical issue)
result := {"severity": "medium", "source": "rego-policy"} if {
	input.signal.severity in ["Sev3", "SEV3", "sev3"]
}

# Enterprise Sev4 → Low (informational)
result := {"severity": "low", "source": "rego-policy"} if {
	input.signal.severity in ["Sev4", "SEV4", "sev4"]
}

# Fallback: unmapped severity → unknown
default result := {"severity": "unknown", "source": "fallback"}
