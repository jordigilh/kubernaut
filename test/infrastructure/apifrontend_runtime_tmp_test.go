package infrastructure

import (
	"context"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("API Frontend E2E runtime filesystem", func() {
	It("UT-INFRA-AF-1807-002 mounts writable /tmp for ambient CA bundle creation", func() {
		manifest := captureKubectlManifest(func() error {
			return deployAPIFrontendService(context.Background(), "test-kubeconfig", "kubernaut-system", "localhost/apifrontend:test", true, nil, io.Discard)
		})

		Expect(manifest).To(ContainSubstring("name: tmp\n              mountPath: /tmp"))
		Expect(manifest).To(ContainSubstring("name: tmp\n          emptyDir: {}"))
		expectValidYAMLDocuments(manifest)
	})
})
