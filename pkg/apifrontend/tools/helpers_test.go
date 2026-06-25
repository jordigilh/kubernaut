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

package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("ParseRRID — name-only rr_id format (#E2E-FIX)", func() {

	Describe("UT-AF-RRID-001: rr_id is treated as plain resource name", func() {
		It("should use rr_id as name and namespace from the explicit arg", func() {
			ns, name, err := tools.ParseRRID("rr-deploy-web-001", "kubernaut-system", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("kubernaut-system"))
			Expect(name).To(Equal("rr-deploy-web-001"))
		})
	})

	Describe("UT-AF-RRID-002: rr_id takes precedence over explicit name", func() {
		It("should ignore the explicit name when rr_id is provided", func() {
			ns, name, err := tools.ParseRRID("rr-from-rrid", "ns-1", "rr-from-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("rr-from-rrid"))
			Expect(ns).To(Equal("ns-1"))
		})
	})

	Describe("UT-AF-RRID-003: empty rr_id falls back to explicit name", func() {
		It("should use namespace and name when rr_id is empty", func() {
			ns, name, err := tools.ParseRRID("", "payments", "rr-fallback-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("payments"))
			Expect(name).To(Equal("rr-fallback-001"))
		})
	})

	Describe("UT-AF-RRID-004: empty rr_id and empty name returns error", func() {
		It("should require name when rr_id is not provided", func() {
			_, _, err := tools.ParseRRID("", "ns", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name is required"))
		})
	})

	Describe("UT-AF-RRID-005: rr_id containing slash is passed through as name (no split)", func() {
		It("should NOT split on slash — rr_id is always the full name", func() {
			ns, name, err := tools.ParseRRID("has/slash", "ns", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("has/slash"),
				"ParseRRID must not split; validation of K8s name rules is a separate concern")
			Expect(ns).To(Equal("ns"))
		})
	})

	Describe("UT-AF-RRID-006: namespace is passed through from explicit arg", func() {
		It("should preserve the namespace argument as-is", func() {
			ns, _, err := tools.ParseRRID("rr-001", "custom-ns", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("custom-ns"))
		})
	})

	Describe("UT-AF-RRID-007: empty namespace with valid rr_id returns empty namespace", func() {
		It("should not error when namespace is empty — callers set defaults", func() {
			ns, name, err := tools.ParseRRID("rr-no-ns", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal(""))
			Expect(name).To(Equal("rr-no-ns"))
		})
	})
})

var _ = Describe("ParseRARID — tolerant rar_id parser (#1493)", func() {

	Describe("UT-AF-1493-001: bare name resolves with injected namespace", func() {
		It("should pair bare rar_id with the explicit namespace argument", func() {
			ns, name, err := tools.ParseRARID("rar-oom-1", "payments", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("payments"))
			Expect(name).To(Equal("rar-oom-1"))
		})
	})

	Describe("UT-AF-1493-002: namespace/name format is accepted and split", func() {
		It("should split on slash when rar_id contains namespace/name", func() {
			ns, name, err := tools.ParseRARID("payments/rar-oom-1", "injected-ns", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("payments"))
			Expect(name).To(Equal("rar-oom-1"))
		})
	})

	Describe("UT-AF-1493-003: empty rar_id falls back to explicit namespace + name", func() {
		It("should use explicit args when rar_id is empty", func() {
			ns, name, err := tools.ParseRARID("", "pay", "rar-fallback-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("pay"))
			Expect(name).To(Equal("rar-fallback-1"))
		})
	})

	Describe("UT-AF-1493-004: empty rar_id + empty name returns error", func() {
		It("should require name when rar_id is not provided", func() {
			_, _, err := tools.ParseRARID("", "ns", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name is required"))
		})
	})
})

var _ = Describe("IsTerminalPhase", func() {
	DescribeTable("UT-AF-TERM-001: classifies phases correctly",
		func(phase string, expected bool) {
			Expect(tools.IsTerminalPhase(phase)).To(Equal(expected))
		},
		Entry("Completed is terminal", "Completed", true),
		Entry("Failed is terminal", "Failed", true),
		Entry("Cancelled is terminal", "Cancelled", true),
		Entry("Pending is non-terminal", "Pending", false),
		Entry("Executing is non-terminal", "Executing", false),
		Entry("Analyzing is non-terminal", "Analyzing", false),
		Entry("empty is non-terminal", "", false),
	)
})
