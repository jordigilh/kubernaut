//go:build e2e
// +build e2e

package dataanalytics

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DATA-ANALYTICS-E2E-SUITE-001: Data Management and Analytics E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of data collection, processing, and analytics for business intelligence
// Stakeholder Value: Executive confidence in data-driven decision making, business insights, and strategic planning capabilities

func TestDataAnalyticsE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Management and Analytics E2E Business Suite")
}
