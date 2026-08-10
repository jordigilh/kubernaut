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
	"context"
	"errors"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("kubernaut_remediate (#1332 Intent-Based Tool Redesign)", func() {
	Describe("HandleRemediate — RR creation without IS (F-01)", func() {
		It("UT-AF-1332-001: creates RR with valid namespace/kind/name and returns rr_id", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web")}, &tools.RemediateArgs{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web",
				Description: "Pod CrashLoopBackOff detected",
				APIVersion:  "apps/v1",
			}, "sre-user")

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.RRID).To(HavePrefix("rr-"))
			Expect(result.AlreadyExists).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("created"))
		})

		It("UT-AF-1332-002: deduplication returns already_exists for same fingerprint", func() {
			tc := newTypedFakeClient()

			result1, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web")}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "first", APIVersion: "apps/v1",
			}, "user-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.AlreadyExists).To(BeFalse())

			result2, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web")}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "second", APIVersion: "apps/v1",
			}, "user-b")
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.AlreadyExists).To(BeTrue())
			Expect(result2.RRID).To(Equal(result1.RRID))
		})

		It("UT-AF-1332-003: accepts empty namespace for cluster-scoped resources (#1372)", func() {
			tc := newTypedFakeClient()
			result, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("", "Node", "worker-1")}, &tools.RemediateArgs{
				Namespace: "", Kind: "Node", Name: "worker-1", APIVersion: "v1",
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1332-004: rejects empty kind", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web")}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "", Name: "web", APIVersion: "apps/v1",
			}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1332-005: rejects empty name", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web")}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "", APIVersion: "apps/v1",
			}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1332-006: returns ErrK8sUnavailable when client is nil", func() {
			_, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: nil, ControllerNS: "kubernaut-system"}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", APIVersion: "apps/v1",
			}, "user")
			Expect(err).To(MatchError(tools.ErrK8sUnavailable))
		})

		It("UT-AF-1332-007 / UT-AF-1839-011: nil Triager (severityTriage.enabled=false) fails closed instead of fabricating a severity", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system"}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-sev", APIVersion: "apps/v1",
			}, "user")
			Expect(errors.Is(err, severity.ErrSeverityUndetermined)).To(BeTrue(),
				"#1839/DD-AF-010: a nil Triager must fail closed like 'no evidence found', not silently default to warning")
			Expect(result.RRID).To(BeEmpty())
		})

		It("UT-AF-1332-008: existing rr_id path looks up RR status (fixes status.phase bug)", func() {
			tc := newTypedFakeClient()

			createResult, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "existing-target")}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "existing-target", APIVersion: "apps/v1",
			}, "user")
			Expect(err).NotTo(HaveOccurred())

			lookupResult, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "existing-target")}, &tools.RemediateArgs{
				RRID: createResult.RRID,
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(lookupResult.RRID).To(Equal(createResult.RRID))
			Expect(lookupResult.AlreadyExists).To(BeTrue())
		})
	})

	Describe("NewRemediateTool — tool constructor (F-06)", func() {
		It("UT-AF-1332-009: creates tool with name kubernaut_remediate", func() {
			tc := newTypedFakeClient()
			t, err := tools.NewRemediateTool(tc, nil, "kubernaut-system", nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Name()).To(Equal("kubernaut_remediate"))
		})
	})

	Describe("APIVersion support (#1372)", func() {
		It("UT-AF-1372-070: remediate with api_version populated -> RR has apiVersion set", func() {
			tc := newTypedFakeClient()
			result, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web-apiver")}, &tools.RemediateArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web-apiver",
				APIVersion: "apps/v1",
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1372-071: remediate with empty api_version rejects", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web")}, &tools.RemediateArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web",
				APIVersion: "",
			}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("api_version"))
		})
	})

	Describe("RR context enrichment — #1423 (AU-3, SI-4)", func() {
		It("UT-AF-1423-020: HandleRemediate sets RR context on EventBridge after RR creation", func() {
			tc := newTypedFakeClient()
			q := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), q, a2a.NewTaskID(), "ctx-1423-020", nil)

			result, err := tools.HandleRemediate(ctx, &tools.ToolDeps{Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web-enriched")}, &tools.RemediateArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web-enriched",
				APIVersion: "apps/v1",
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())

			Expect(launcher.EmitStatusSafe(ctx, "post-remediate status")).To(Succeed())

			found := false
			for _, evt := range q.Events() {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				meta := statusEvt.Metadata
				if meta == nil {
					continue
				}
				if rrid, ok := meta["rr_id"].(string); ok && rrid == result.RRID {
					found = true
					Expect(meta["namespace"]).To(Equal("prod"),
						"AU-3: namespace must be present for audit trail correlation")
					Expect(meta["kind"]).To(Equal("Deployment"))
					Expect(meta["target"]).To(Equal("Deployment/web-enriched"),
						"AU-3: target must use Kind/Name format for unambiguous resource identification in audit trail")
					Expect(meta["phase"]).To(Equal("Investigating"),
						"SI-4: initial phase must be Investigating")
				}
			}
			Expect(found).To(BeTrue(),
				"AU-3: status events after HandleRemediate must carry rr_id from RR context")
		})
	})

	Describe("Fleet cluster_id end-to-end wiring (#1409)", func() {
		It("IT-AF-1409-002: AU-3, SI-4 — kubernaut_remediate correctly attributes and correlates cluster identity end-to-end", func() {
			tc := newTypedFakeClient()
			rec := &auditRecorder{}
			q := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), q, a2a.NewTaskID(), "ctx-it-1409-002", nil)

			result, err := tools.HandleRemediate(ctx, &tools.ToolDeps{
				Client:       tc,
				ControllerNS: "kubernaut-system",
				Auditor:      rec,
				Triager:      defaultTestTriager("prod", "Deployment", "web-fleet"),
			}, &tools.RemediateArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web-fleet",
				APIVersion: "apps/v1",
				ClusterID:  "cluster-fleet-it-002",
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.ClusterID).To(Equal("cluster-fleet-it-002"))

			created := verifyTypedRR(tc, "kubernaut-system", extractRRName(result.RRID))
			Expect(created.Spec.ClusterID).To(Equal("cluster-fleet-it-002"),
				"AU-3, SI-4: cluster_id must reach the RR spec for cross-cluster correlation")

			var auditSaw bool
			for _, e := range rec.events {
				if e.Detail["cluster_id"] == "cluster-fleet-it-002" {
					auditSaw = true
					break
				}
			}
			Expect(auditSaw).To(BeTrue(),
				"AU-3: emitCreateRRAudit's Detail map must attribute cluster_id for the AF-originated fleet RR")

			Expect(launcher.EmitStatusSafe(ctx, "post-remediate status")).To(Succeed())
			var eventSaw bool
			for _, evt := range q.Events() {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok || statusEvt.Metadata == nil {
					continue
				}
				if statusEvt.Metadata["cluster_id"] == "cluster-fleet-it-002" {
					eventSaw = true
				}
			}
			Expect(eventSaw).To(BeTrue(),
				"AU-3, SI-4: A2A status events must carry cluster_id from RRContext for Console cross-cluster correlation")
		})
	})

	// #2025 (main-tracking clone of #2022): HandleRemediate forwards
	// ToolDeps.ScopeChecker straight into HandleCreateRR, so this proves
	// kubernaut_remediate's new-RR path rejects out-of-scope resources.
	Describe("ScopeChecker pre-check (#2025)", func() {
		It("UT-AF-2025-030: rejects an unmanaged target resource before RR creation", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{
				Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web-unmanaged"),
				ScopeChecker: &mocks.NeverManagedScopeChecker{},
			}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-unmanaged", APIVersion: "apps/v1",
			}, "user")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrResourceNotManaged)).To(BeTrue())
		})

		It("UT-AF-2025-031: allows RR creation for a managed target resource", func() {
			tc := newTypedFakeClient()
			result, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{
				Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web-managed"),
				ScopeChecker: &mocks.AlwaysManagedScopeChecker{},
			}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-managed", APIVersion: "apps/v1",
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-2025-032: the rr_id (dedup lookup) path is not scope-checked", func() {
			tc := newTypedFakeClient()
			created, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{
				Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web-lookup"),
			}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-lookup", APIVersion: "apps/v1",
			}, "user")
			Expect(err).NotTo(HaveOccurred())

			lookupResult, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{
				Client: tc, ControllerNS: "kubernaut-system", Triager: defaultTestTriager("prod", "Deployment", "web-lookup"),
				ScopeChecker: &mocks.NeverManagedScopeChecker{},
			}, &tools.RemediateArgs{
				RRID: created.RRID,
			}, "user")
			Expect(err).NotTo(HaveOccurred(),
				"the rr_id lookup path reads an already-created (already scope-checked) RR and must not re-run scope validation")
			Expect(lookupResult.RRID).To(Equal(created.RRID))
		})
	})

	// #2025 (main-tracking clone of #2022): HandleRemediate forwards
	// ToolDeps.ScopeChecker straight into HandleCreateRR, so this proves
	// kubernaut_remediate's ambiguous-severity path mirrors af_create_rr's.
	Describe("Ambiguous severity correlation — DD-AF-012 (#2027/#2028)", func() {
		It("UT-AF-2028-005: surfaces Ambiguous/CandidateSignalName/CandidateSeverity when only a cluster-scoped alert correlates, then proceeds once confirmed", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{
				Client: tc, ControllerNS: "kubernaut-system", Triager: ambiguousTestTriager(),
			}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-ambiguous", APIVersion: "apps/v1",
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ambiguous).To(BeTrue())
			Expect(result.CandidateSignalName).To(Equal("TestDefaultAlert"))
			Expect(result.CandidateSeverity).To(Equal("warning"))
			Expect(result.RRID).To(BeEmpty())

			confirmed, err := tools.HandleRemediate(context.Background(), &tools.ToolDeps{
				Client: tc, ControllerNS: "kubernaut-system", Triager: ambiguousTestTriager(),
			}, &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-ambiguous", APIVersion: "apps/v1",
				ConfirmedSignalName: "TestDefaultAlert",
			}, "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(confirmed.Ambiguous).To(BeFalse())
			Expect(confirmed.RRID).NotTo(BeEmpty())
		})
	})
})
