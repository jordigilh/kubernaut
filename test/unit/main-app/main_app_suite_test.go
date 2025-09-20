package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMainApplicationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Application Integration Suite")
}
