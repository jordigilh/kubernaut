//go:build unit
// +build unit

package dependency_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDependency(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dependency Manager Suite")
}
