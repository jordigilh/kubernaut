//go:build unit
// +build unit

package k8s

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-K8S-SUITE-001: Kubernetes Platform Test Suite Organization
// Business Impact: Ensures comprehensive validation of Kubernetes platform business logic
// Stakeholder Value: Provides executive confidence in Kubernetes operations and cluster management

func TestK8s(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernetes Platform Unit Tests Suite")
}



