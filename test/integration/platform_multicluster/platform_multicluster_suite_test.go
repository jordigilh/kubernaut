//go:build integration
// +build integration

package platform_multicluster

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPlatformMultiClusterIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platform Multi-Cluster Integration Test Suite")
}
