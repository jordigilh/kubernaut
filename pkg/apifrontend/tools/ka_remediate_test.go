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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("kubernaut_remediate (#1332 Intent-Based Tool Redesign)", func() {
	Describe("HandleRemediate — RR creation without IS (F-01)", func() {
		It("UT-AF-1332-001: creates RR with valid namespace/kind/name and returns rr_id", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web",
				Description: "Pod CrashLoopBackOff detected",
				APIVersion:  "apps/v1",
			}, "sre-user", defaultTestTriager("prod", "Deployment", "web"), nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(result.RRID).To(HavePrefix("rr-"))
			Expect(result.AlreadyExists).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("created"))
		})

		It("UT-AF-1332-002: deduplication returns already_exists for same fingerprint", func() {
			tc := newTypedFakeClient()

			result1, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "first", APIVersion: "apps/v1",
			}, "user-a", defaultTestTriager("prod", "Deployment", "web"), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.AlreadyExists).To(BeFalse())

			result2, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "second", APIVersion: "apps/v1",
			}, "user-b", defaultTestTriager("prod", "Deployment", "web"), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.AlreadyExists).To(BeTrue())
			Expect(result2.RRID).To(Equal(result1.RRID))
		})

		It("UT-AF-1332-003: accepts empty namespace for cluster-scoped resources (#1372)", func() {
			tc := newTypedFakeClient()
			result, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "", Kind: "Node", Name: "worker-1", APIVersion: "v1",
			}, "user", defaultTestTriager("", "Node", "worker-1"), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1332-004: rejects empty kind", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "", Name: "web", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1332-005: rejects empty name", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1332-006: returns ErrK8sUnavailable when client is nil", func() {
			_, err := tools.HandleRemediate(context.Background(), nil, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(err).To(MatchError(tools.ErrK8sUnavailable))
		})

		It("UT-AF-1332-007 / UT-AF-1839-011: nil Triager (severityTriage.enabled=false) fails closed instead of fabricating a severity", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-sev", APIVersion: "apps/v1",
			}, "user", nil, nil)
			Expect(errors.Is(err, severity.ErrSeverityUndetermined)).To(BeTrue(),
				"#1839/DD-AF-010: a nil Triager must fail closed like 'no evidence found', not silently default to warning")
			Expect(result.RRID).To(BeEmpty())
		})

		It("UT-AF-1332-008: existing rr_id path looks up RR status (fixes status.phase bug)", func() {
			tc := newTypedFakeClient()

			createResult, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "existing-target", APIVersion: "apps/v1",
			}, "user", defaultTestTriager("prod", "Deployment", "existing-target"), nil)
			Expect(err).NotTo(HaveOccurred())

			lookupResult, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				RRID: createResult.RRID,
			}, "user", defaultTestTriager("", "", ""), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(lookupResult.RRID).To(Equal(createResult.RRID))
			Expect(lookupResult.AlreadyExists).To(BeTrue())
		})
	})

	Describe("Ambiguous severity/signal correlation — DD-AF-012 (#2027)", func() {
		It("UT-AF-2027-005: surfaces Ambiguous/CandidateSignalName/CandidateSeverity when only a cluster-scoped alert correlates, then proceeds once confirmed", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-ambiguous", APIVersion: "apps/v1",
			}, "user", ambiguousTestTriager(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ambiguous).To(BeTrue())
			Expect(result.CandidateSignalName).To(Equal("TestDefaultAlert"))
			Expect(result.CandidateSeverity).To(Equal("warning"))
			Expect(result.RRID).To(BeEmpty())

			confirmed, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web-ambiguous", APIVersion: "apps/v1",
				ConfirmedSignalName: "TestDefaultAlert",
			}, "user", ambiguousTestTriager(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(confirmed.Ambiguous).To(BeFalse())
			Expect(confirmed.RRID).NotTo(BeEmpty())
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
			result, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web-apiver",
				APIVersion: "apps/v1",
			}, "user", defaultTestTriager("prod", "Deployment", "web-apiver"), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-1372-071: remediate with empty api_version rejects", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web",
				APIVersion: "",
			}, "user", nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("api_version"))
		})
	})

	Describe("RR context enrichment — #1423 (AU-3, SI-4)", func() {
		It("UT-AF-1423-020: HandleRemediate sets RR context on EventBridge after RR creation", func() {
			tc := newTypedFakeClient()
			q := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), q, a2a.NewTaskID(), "ctx-1423-020", nil)

			result, err := tools.HandleRemediate(ctx, tc, nil, "kubernaut-system", &tools.RemediateArgs{
				Namespace:  "prod",
				Kind:       "Deployment",
				Name:       "web-enriched",
				APIVersion: "apps/v1",
			}, "user", defaultTestTriager("prod", "Deployment", "web-enriched"), nil)
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
					Expect(meta["target"]).To(Equal("web-enriched"))
					Expect(meta["phase"]).To(Equal("Investigating"),
						"SI-4: initial phase must be Investigating")
				}
			}
			Expect(found).To(BeTrue(),
				"AU-3: status events after HandleRemediate must carry rr_id from RR context")
		})
	})

	Describe("Scope validation before RR creation — #2022 (AC-6, SI-10, AU-3/AU-12, SI-11)", func() {
		remediateArgs := func() *tools.RemediateArgs {
			return &tools.RemediateArgs{
				Namespace:  "unmanaged-ns",
				Kind:       "Deployment",
				Name:       "legacy-app",
				APIVersion: "apps/v1",
			}
		}

		It("UT-AF-2022-010: rejects an unmanaged resource without creating an RR", func() {
			tc := newTypedFakeClient()
			ctx := tools.ContextWithScopeChecker(context.Background(), &mocks.NeverManagedScopeChecker{})

			result, err := tools.HandleRemediate(ctx, tc, nil, "kubernaut-system", remediateArgs(), "sre-bob", defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"), nil)

			Expect(err).NotTo(HaveOccurred(), "SI-11: rejection must be a clear result, not an opaque tool error")
			Expect(result.Managed).To(BeFalse())
			Expect(result.RRID).To(BeEmpty())
			Expect(result.Message).To(ContainSubstring("not managed by Kubernaut"))
		})

		It("UT-AF-2022-011: allows a managed resource to proceed to RR creation", func() {
			tc := newTypedFakeClient()
			ctx := tools.ContextWithScopeChecker(context.Background(), &mocks.AlwaysManagedScopeChecker{})

			result, err := tools.HandleRemediate(ctx, tc, nil, "kubernaut-system", remediateArgs(), "sre-bob", defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"), nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Managed).To(BeTrue())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-2022-012: no ScopeChecker in context gracefully degrades to always-managed (backward compat)", func() {
			tc := newTypedFakeClient()

			result, err := tools.HandleRemediate(context.Background(), tc, nil, "kubernaut-system", remediateArgs(), "sre-bob", defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"), nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Managed).To(BeTrue())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-2022-013: emits an EventRRScopeRejected audit event on rejection (AU-3/AU-12)", func() {
			tc := newTypedFakeClient()
			recorder := &auditRecorder{}
			ctx := tools.ContextWithScopeChecker(context.Background(), &mocks.NeverManagedScopeChecker{})

			_, err := tools.HandleRemediate(ctx, tc, nil, "kubernaut-system", remediateArgs(), "sre-bob", defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"), recorder)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.events).To(HaveLen(1))
			Expect(recorder.events[0].Type).To(Equal(audit.EventRRScopeRejected))
			Expect(recorder.events[0].UserID).To(Equal("sre-bob"))
		})
	})
})
