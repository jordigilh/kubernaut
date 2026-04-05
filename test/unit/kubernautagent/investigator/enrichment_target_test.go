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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("TP-433-ADV P4: Enrichment Target Resolution — GAP-001", func() {

	Describe("UT-KA-433-ENR-001: ResolveEnrichmentTarget prefers RCA target over signal", func() {
		It("should use RCA remediation target when populated", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				Name:         "api-server-abc123",
				Namespace:    "production",
			}
			rcaResult := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: "production",
				},
			}

			kind, name, ns := investigator.ResolveEnrichmentTarget(signal, rcaResult)
			Expect(kind).To(Equal("Deployment"))
			Expect(name).To(Equal("api-server"))
			Expect(ns).To(Equal("production"))
		})
	})

	Describe("UT-KA-433-ENR-002: ResolveEnrichmentTarget falls back to signal when RCA target empty", func() {
		It("should fall back to signal resource when RCA target is empty", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				Name:         "OOMKilled",
				ResourceName: "api-server-abc123",
				Namespace:    "production",
			}
			rcaResult := &katypes.InvestigationResult{
				RCASummary: "Something went wrong",
			}

			kind, name, ns := investigator.ResolveEnrichmentTarget(signal, rcaResult)
			Expect(kind).To(Equal("Pod"))
			Expect(name).To(Equal("api-server-abc123"))
			Expect(ns).To(Equal("production"))
		})
	})

	Describe("UT-KA-433-ENR-003: ResolveEnrichmentTarget falls back to signal when rcaResult is nil", func() {
		It("should fall back to signal when rcaResult is nil", func() {
			signal := katypes.SignalContext{
				ResourceKind: "StatefulSet",
				Name:         "HighMemory",
				ResourceName: "redis-cluster",
				Namespace:    "cache",
			}

			kind, name, ns := investigator.ResolveEnrichmentTarget(signal, nil)
			Expect(kind).To(Equal("StatefulSet"))
			Expect(name).To(Equal("redis-cluster"))
			Expect(ns).To(Equal("cache"))
		})
	})

	Describe("UT-KA-433-ENR-004: ResolveEnrichmentTarget defaults empty ResourceKind to Pod", func() {
		It("should default to Pod when signal has no ResourceKind", func() {
			signal := katypes.SignalContext{
				Name:         "OOMKilled",
				ResourceName: "api-server-abc123",
				Namespace:    "production",
			}

			kind, name, ns := investigator.ResolveEnrichmentTarget(signal, nil)
			Expect(kind).To(Equal("Pod"))
			Expect(name).To(Equal("api-server-abc123"))
			Expect(ns).To(Equal("production"))
		})
	})

	// ===== Audit findings =====

	Describe("AUDIT-C1: ResolveEnrichmentTarget uses ResourceName, not Name (SignalName)", func() {
		It("should use ResourceName for enrichment, not SignalName", func() {
			signal := katypes.SignalContext{
				Name:         "OOMKilled",
				ResourceKind: "Pod",
				ResourceName: "web-server-abc123",
				Namespace:    "production",
			}

			kind, name, ns := investigator.ResolveEnrichmentTarget(signal, nil)
			Expect(kind).To(Equal("Pod"))
			Expect(name).To(Equal("web-server-abc123"),
				"C1: must use ResourceName (K8s object), not Name (signal type)")
			Expect(ns).To(Equal("production"))
		})

		It("should fall back to Name only when ResourceName is empty", func() {
			signal := katypes.SignalContext{
				Name:         "OOMKilled",
				ResourceKind: "Pod",
				Namespace:    "production",
			}

			kind, name, ns := investigator.ResolveEnrichmentTarget(signal, nil)
			Expect(kind).To(Equal("Pod"))
			Expect(name).To(Equal("OOMKilled"),
				"when ResourceName is empty, Name is the only fallback")
			Expect(ns).To(Equal("production"))
		})
	})

	Describe("AUDIT-M2: ResolveEnrichmentTarget handles partial RemediationTarget", func() {
		It("should fall back to signal when RCA has Name but no Kind", func() {
			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "web-pod",
				Namespace:    "production",
			}
			rcaResult := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Name:      "api-server",
					Namespace: "production",
				},
			}

			kind, name, ns := investigator.ResolveEnrichmentTarget(signal, rcaResult)
			Expect(kind).To(Equal("Pod"),
				"M2: partial target (no Kind) should fall back to signal")
			Expect(name).To(Equal("web-pod"))
			Expect(ns).To(Equal("production"))
		})
	})
})
