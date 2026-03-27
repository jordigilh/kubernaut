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

package transport_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kapi/config"
	"github.com/jordigilh/kubernaut/pkg/kapi/llm/transport"
)

var _ = Describe("Credential Scrubbing — #417", func() {

	// UT-KAPI-417-009: Credential scrubbing in logs
	Describe("UT-KAPI-417-009: Sensitive header values redacted", func() {
		It("should redact sensitive header value", func() {
			result := transport.RedactHeaderValue("Bearer secret-key", true)
			Expect(result).To(Equal("[REDACTED]"))
		})

		It("should NOT redact non-sensitive header value", func() {
			result := transport.RedactHeaderValue("prod", false)
			Expect(result).To(Equal("prod"))
		})

		It("should identify secretKeyRef as sensitive source", func() {
			def := config.HeaderDefinition{Name: "Authorization", SecretKeyRef: "SECRET_ENV"}
			Expect(transport.IsSensitiveSource(def)).To(BeTrue())
		})

		It("should identify filePath as sensitive source", func() {
			def := config.HeaderDefinition{Name: "Authorization", FilePath: "/var/run/token"}
			Expect(transport.IsSensitiveSource(def)).To(BeTrue())
		})

		It("should identify value as NOT sensitive source", func() {
			def := config.HeaderDefinition{Name: "x-tenant-id", Value: "prod"}
			Expect(transport.IsSensitiveSource(def)).To(BeFalse())
		})
	})
})
