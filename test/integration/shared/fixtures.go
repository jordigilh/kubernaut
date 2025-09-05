package shared

import (
	"github.com/jordigilh/kubernaut/test/integration/fixtures"
)

// Re-export types and variables for compatibility
type TestCase = fixtures.TestCase

var IntegrationTestAlerts = fixtures.IntegrationTestAlerts

// Re-export helper functions
var (
	PerformanceTestAlert      = fixtures.PerformanceTestAlert
	ConcurrentTestAlert       = fixtures.ConcurrentTestAlert
	MalformedAlert            = fixtures.MalformedAlert
	ChaosEngineeringTestAlert = fixtures.ChaosEngineeringTestAlert
	SecurityIncidentAlert     = fixtures.SecurityIncidentAlert
	ResourceExhaustionAlert   = fixtures.ResourceExhaustionAlert
	CascadingFailureAlert     = fixtures.CascadingFailureAlert
	GetAllEdgeCaseAlerts      = fixtures.GetAllEdgeCaseAlerts
)
