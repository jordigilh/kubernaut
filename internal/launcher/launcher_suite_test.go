package launcher_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLauncherSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Launcher Suite")
}
