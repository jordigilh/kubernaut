package integration

import (
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/fixtures"
)

// Re-export types and variables for backward compatibility
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

// Legacy compatibility - these were part of the original large fixtures file
var (
	ChaosEngineeringAlerts         []TestCase
	SecurityComplianceAlerts       []TestCase
	ResourceExhaustionAlerts       []TestCase
	CascadingFailureAlerts         []TestCase
	MultiAlertCorrelationScenarios []TestCase
)
