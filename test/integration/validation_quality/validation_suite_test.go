//go:build integration
// +build integration

package validation_quality

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestValidationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validation Integration Suite")
}
