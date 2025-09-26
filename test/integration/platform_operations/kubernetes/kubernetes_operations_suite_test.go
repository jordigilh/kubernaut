//go:build integration
// +build integration

package kubernetes

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUkubernetesUoperations(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "kubernetes operations Suite")
}
