package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSetupDemoInfra(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Demo Infra Suite")
}
