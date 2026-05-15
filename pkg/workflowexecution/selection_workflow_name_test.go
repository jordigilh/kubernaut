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

package workflowexecution_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
)

var _ = Describe("Issue #1033 Gap 2: workflow_name in selection audit (BR-AUDIT-005)", func() {

	var (
		ctx   context.Context
		store *mockAuditStore
		mgr   *weaudit.Manager
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = &mockAuditStore{}
		logger := zap.New(zap.UseDevMode(true))
		mgr = weaudit.NewManager(store, logger)
	})

	wfe := func(name string) *workflowexecutionv1alpha1.WorkflowExecution {
		return newTestWFE(name, "default", "default/Deployment/test-app", "wf-test-123", "quay.io/kubernaut/test:v1", nil)
	}

	// ========================================
	// P0: workflow_name present when provided
	// ========================================
	Describe("workflow_name presence (P0)", func() {

		It("UT-WE-1033-001: workflow name provided → audit payload includes workflow_name", func() {
			wfeObj := wfe("wfe-with-name")

			err := mgr.RecordWorkflowSelectionCompleted(ctx, wfeObj, "fix-security-context-job")
			Expect(err).ToNot(HaveOccurred())
			Expect(store.events).To(HaveLen(1))

			payload, ok := store.events[0].EventData.GetWorkflowExecutionAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.WorkflowName.IsSet()).To(BeTrue(),
				"workflow_name should be set when provided")
			Expect(payload.WorkflowName.Value).To(Equal("fix-security-context-job"))
		})

		It("UT-WE-1033-002: workflow name empty → audit payload omits workflow_name", func() {
			wfeObj := wfe("wfe-no-name")

			err := mgr.RecordWorkflowSelectionCompleted(ctx, wfeObj, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(store.events).To(HaveLen(1))

			payload, ok := store.events[0].EventData.GetWorkflowExecutionAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.WorkflowName.IsSet()).To(BeFalse(),
				"workflow_name should NOT be set when empty string passed")
		})
	})

	// ========================================
	// P1: Table-driven workflow name variants
	// ========================================
	Describe("workflow_name variants (P1)", func() {

		It("UT-WE-1033-003: table-driven — various workflow names serialized correctly", func() {
			names := []string{
				"fix-security-context-job",
				"a",
				strings.Repeat("long-name-", 25),
				"workflow-with-unicode-名前",
				"hyphenated-multi-word-name",
			}

			for _, name := range names {
				store.events = nil
				wfeObj := wfe("wfe-variant")

				err := mgr.RecordWorkflowSelectionCompleted(ctx, wfeObj, name)
				Expect(err).ToNot(HaveOccurred())
				Expect(store.events).To(HaveLen(1))

				payload, ok := store.events[0].EventData.GetWorkflowExecutionAuditPayload()
				Expect(ok).To(BeTrue())
				Expect(payload.WorkflowName.IsSet()).To(BeTrue(),
					"workflow_name should be set for: %s", name)
				Expect(payload.WorkflowName.Value).To(Equal(name),
					"workflow_name should match for: %s", name)
			}
		})

		It("UT-WE-1033-004: schemaMeta nil → empty name passed → workflow_name omitted", func() {
			wfeObj := wfe("wfe-nil-schema")

			err := mgr.RecordWorkflowSelectionCompleted(ctx, wfeObj, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(store.events).To(HaveLen(1))

			payload, ok := store.events[0].EventData.GetWorkflowExecutionAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.WorkflowName.IsSet()).To(BeFalse(),
				"workflow_name should NOT be set when schemaMeta is nil")
		})

		It("UT-WE-1033-005: schemaMeta non-nil, WorkflowName empty → workflow_name omitted", func() {
			wfeObj := wfe("wfe-empty-schema-name")

			err := mgr.RecordWorkflowSelectionCompleted(ctx, wfeObj, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(store.events).To(HaveLen(1))

			payload, ok := store.events[0].EventData.GetWorkflowExecutionAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.WorkflowName.IsSet()).To(BeFalse(),
				"workflow_name should NOT be set when WorkflowName is empty")
		})
	})

	// ========================================
	// P1: StoreAudit error path (QE-2)
	// ========================================
	Describe("StoreAudit error path (P1)", func() {

		It("UT-WE-1033-007: StoreAudit failure → wrapped error returned (ADR-032)", func() {
			store.err = fmt.Errorf("connection refused")
			wfeObj := wfe("wfe-err-path")

			err := mgr.RecordWorkflowSelectionCompleted(ctx, wfeObj, "some-workflow")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mandatory audit write failed per ADR-032"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
			Expect(store.events).To(BeEmpty(), "no event should be stored when StoreAudit fails")
		})
	})

	// ========================================
	// P2: Adversarial workflow names
	// ========================================
	Describe("Adversarial workflow names (P2)", func() {

		It("UT-WE-1033-006: adversarial names do not panic and are passed through", func() {
			adversarial := []struct {
				name  string
				value string
			}{
				{name: "max-length+1", value: strings.Repeat("a", 256)},
				{name: "path traversal", value: "../../etc/passwd"},
				{name: "null bytes", value: "fix-sec\x00ctx-job"},
				{name: "CJK characters", value: "名前"},
				{name: "zero-width space", value: "fix\u200bjob"},
			}

			for _, tc := range adversarial {
				store.events = nil
				wfeObj := wfe("wfe-adv")

				err := mgr.RecordWorkflowSelectionCompleted(ctx, wfeObj, tc.value)
				Expect(err).ToNot(HaveOccurred(), "should not panic for %s", tc.name)
				Expect(store.events).To(HaveLen(1))

				payload, ok := store.events[0].EventData.GetWorkflowExecutionAuditPayload()
				Expect(ok).To(BeTrue())
				Expect(payload.WorkflowName.IsSet()).To(BeTrue(),
					"workflow_name should be set for adversarial input: %s", tc.name)
				Expect(payload.WorkflowName.Value).To(Equal(tc.value),
					"workflow_name should passthrough for: %s", tc.name)
			}
		})
	})
})
