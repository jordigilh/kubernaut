//go:build integration
// +build integration

package kubernetes

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PLAT-K8S-001: Platform Kubernetes Operations Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Platform Kubernetes Operations business logic
// Stakeholder Value: Provides executive confidence in Platform Kubernetes Operations testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Platform Kubernetes Operations capabilities
// Business Impact: Ensures all Platform Kubernetes Operations components deliver measurable system reliability
// Business Outcome: Test suite framework enables Platform Kubernetes Operations validation

func TestUkubernetes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platform Kubernetes Operations Suite")
}
