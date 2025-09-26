//go:build integration
// +build integration

package multi_provider

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUmultiUproviderUai(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "multi provider ai Suite")
}
