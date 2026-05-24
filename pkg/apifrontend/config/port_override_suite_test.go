package config_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPortOverrideSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Port Override Suite")
}
