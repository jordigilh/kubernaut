//go:build unit
// +build unit

package performance

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConcurrentLoadComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Concurrent Load Comprehensive Unit Test Suite")
}
