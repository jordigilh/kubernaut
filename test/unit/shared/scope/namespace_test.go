/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scope_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// BR-SCOPE-001: Resource Scope Management
// ADR-057: CRD Namespace Consolidation — controller namespace discovery
var _ = Describe("GetControllerNamespace [ADR-057]", func() {

	var (
		origEnv     string
		origPath    string
		envWasSet   bool
	)

	BeforeEach(func() {
		origEnv, envWasSet = os.LookupEnv("KUBERNAUT_CONTROLLER_NAMESPACE")
		origPath = scope.NamespaceFilePath
		os.Unsetenv("KUBERNAUT_CONTROLLER_NAMESPACE")
	})

	AfterEach(func() {
		if envWasSet {
			os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", origEnv)
		} else {
			os.Unsetenv("KUBERNAUT_CONTROLLER_NAMESPACE")
		}
		scope.NamespaceFilePath = origPath
	})

	Context("UT-NS-057-001: environment variable override", func() {
		It("returns the namespace from KUBERNAUT_CONTROLLER_NAMESPACE when set", func() {
			os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "my-custom-ns")

			ns, err := scope.GetControllerNamespace()
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("my-custom-ns"))
		})

		It("trims whitespace from the environment variable value", func() {
			os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "  kubernaut-system  \n")

			ns, err := scope.GetControllerNamespace()
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("kubernaut-system"))
		})

		It("returns error when environment variable is set to empty string", func() {
			os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "")

			_, err := scope.GetControllerNamespace()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty"))
		})
	})

	Context("UT-NS-057-002: service account file fallback", func() {
		It("reads namespace from the service account file when env var is not set", func() {
			tmpDir := GinkgoT().TempDir()
			nsFile := filepath.Join(tmpDir, "namespace")
			Expect(os.WriteFile(nsFile, []byte("kubernaut-system"), 0644)).To(Succeed())
			scope.NamespaceFilePath = nsFile

			ns, err := scope.GetControllerNamespace()
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("kubernaut-system"))
		})

		It("trims whitespace from the file contents", func() {
			tmpDir := GinkgoT().TempDir()
			nsFile := filepath.Join(tmpDir, "namespace")
			Expect(os.WriteFile(nsFile, []byte("  kubernaut-system\n"), 0644)).To(Succeed())
			scope.NamespaceFilePath = nsFile

			ns, err := scope.GetControllerNamespace()
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("kubernaut-system"))
		})

		It("returns error when the file does not exist and env var is not set", func() {
			scope.NamespaceFilePath = "/nonexistent/path/namespace"

			_, err := scope.GetControllerNamespace()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("controller namespace"))
		})

		It("returns error when the file content is empty", func() {
			tmpDir := GinkgoT().TempDir()
			nsFile := filepath.Join(tmpDir, "namespace")
			Expect(os.WriteFile(nsFile, []byte(""), 0644)).To(Succeed())
			scope.NamespaceFilePath = nsFile

			_, err := scope.GetControllerNamespace()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty"))
		})
	})

	Context("UT-NS-057-003: precedence", func() {
		It("prefers env var over file when both are available", func() {
			os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "env-namespace")

			tmpDir := GinkgoT().TempDir()
			nsFile := filepath.Join(tmpDir, "namespace")
			Expect(os.WriteFile(nsFile, []byte("file-namespace"), 0644)).To(Succeed())
			scope.NamespaceFilePath = nsFile

			ns, err := scope.GetControllerNamespace()
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("env-namespace"))
		})
	})
})
