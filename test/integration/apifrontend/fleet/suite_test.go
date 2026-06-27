package fleet_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAFFleet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AF Fleet Integration Suite")
}
