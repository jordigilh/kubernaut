package parentlifecycle_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestParentLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ParentLifecycle Suite")
}
