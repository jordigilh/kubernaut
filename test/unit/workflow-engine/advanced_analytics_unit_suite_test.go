//go:build unit
// +build unit

package workflowengine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAdvancedAnalyticsUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Advanced Analytics Unit Suite")
}
