//go:build integration
// +build integration

package alert_processing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestAlertProcessing runs the Phase 1 Alert Processing Performance integration test suite
func TestAlertProcessing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase 1: Alert Processing Performance Suite")
}