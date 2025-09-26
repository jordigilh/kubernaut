//go:build integration
// +build integration

package monitoring

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUhealthUmonitoring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "health monitoring Suite")
}
