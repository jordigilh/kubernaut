//go:build unit
// +build unit

package feedback

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFeedbackLoopComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Feedback Loop Comprehensive Unit Test Suite")
}
