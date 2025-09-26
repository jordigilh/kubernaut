//go:build integration
// +build integration

package multicluster

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUplatformUmulticluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "platform multicluster Suite")
}
