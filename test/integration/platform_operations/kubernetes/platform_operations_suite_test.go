//go:build integration
// +build integration

package kubernetes

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUplatformUoperations(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "platform operations Suite")
}
