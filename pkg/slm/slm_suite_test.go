package slm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSlm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Slm Suite")
}
