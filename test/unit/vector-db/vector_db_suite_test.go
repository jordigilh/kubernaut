package vectordb

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVectorDatabaseProductionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Vector Database Production Integration Suite")
}
