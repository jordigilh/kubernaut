/*
Copyright 2026 Jordi Gil.

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

package infrastructure

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("validateImageArchive", func() {
	It("UT-INFRA-2443-001: accepts valid image archives below the former 100 MB threshold", func() {
		tempDir, err := os.MkdirTemp("", "image-archive-test-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, tempDir)

		tarPath := filepath.Join(tempDir, "service.tar")
		Expect(os.WriteFile(tarPath, []byte("valid archive"), 0o600)).To(Succeed())

		size, err := validateImageArchive(tarPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(size).To(Equal(int64(len("valid archive"))))
	})

	It("UT-INFRA-2443-002: rejects empty image archives", func() {
		tempDir, err := os.MkdirTemp("", "image-archive-test-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, tempDir)

		tarPath := filepath.Join(tempDir, "empty.tar")
		Expect(os.WriteFile(tarPath, nil, 0o600)).To(Succeed())

		_, err = validateImageArchive(tarPath)
		Expect(err).To(MatchError(ContainSubstring(".tar file is empty")))
	})
})
