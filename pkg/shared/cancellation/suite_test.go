package cancellation_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCancellation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Cancellation Utilities Suite")
}
