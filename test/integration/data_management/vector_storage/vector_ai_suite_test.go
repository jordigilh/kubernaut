//go:build integration
// +build integration

package vector_storage

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUvectorUai(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vector ai Suite")
}
