//go:build e2e
// +build e2e

package webhook

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WEBHOOK-E2E-001: Webhook Service E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of complete AlertManager → Kubernetes workflows
// Stakeholder Value: Executive confidence in end-to-end alert processing and automated remediation

func TestWebhookE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhook Service E2E Business Suite")
}
