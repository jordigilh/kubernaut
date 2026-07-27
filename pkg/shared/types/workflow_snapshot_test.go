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

package types_test

import (
	"encoding/json"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// UT-SHARED-1661-654 (Issue #1661 Change 12, DD-WORKFLOW-018)
// ========================================
// WorkflowSnapshot is inline-embedded (Go anonymous struct embedding + JSON
// `,inline`) into both AIAnalysis.Status.SelectedWorkflow and
// WorkflowExecution.Spec.WorkflowRef specifically so their field lists can
// never independently drift again -- ActionType was "left off" WorkflowRef's
// hand-copied list once already, and WorkflowName was never wired at all
// until Change 12 closed that gap. This test proves the structural
// invariant the shared type is meant to guarantee: a fully-populated
// WorkflowSnapshot round-trips through JSON with a fixed, complete key set,
// so any future field addition to this one type is automatically present
// wherever it's embedded, with no separate CRD-side edit required.
// ========================================
var _ = Describe("WorkflowSnapshot — Issue #1661 Change 12 shared embed", func() {
	It("UT-SHARED-1661-654-001: round-trips all 12 fields through JSON with the expected wire keys", func() {
		snap := sharedtypes.WorkflowSnapshot{
			WorkflowID:            "wf-oom-recovery",
			WorkflowName:          "oom-recovery",
			ActionType:            "ScaleReplicas",
			Version:               "v1.0.0",
			ExecutionBundle:       "quay.io/kubernaut/oom-recovery:v1",
			ExecutionBundleDigest: "sha256:abc123",
			ExecutionEngine:       "job",
			EngineConfig:          &apiextensionsv1.JSON{Raw: []byte(`{"key":"value"}`)},
			ServiceAccountName:    "kubernaut-workflow-runner",
			Dependencies: &sharedtypes.WorkflowDependencies{
				Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
			},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			},
			DeclaredParameterNames: map[string]bool{"TARGET_NAMESPACE": true},
		}

		raw, err := json.Marshal(snap)
		Expect(err).NotTo(HaveOccurred())

		var asMap map[string]interface{}
		Expect(json.Unmarshal(raw, &asMap)).To(Succeed())

		keys := make([]string, 0, len(asMap))
		for k := range asMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		Expect(keys).To(Equal([]string{
			"actionType",
			"declaredParameterNames",
			"dependencies",
			"engineConfig",
			"executionBundle",
			"executionBundleDigest",
			"executionEngine",
			"resources",
			"serviceAccountName",
			"version",
			"workflowId",
			"workflowName",
		}), "WorkflowSnapshot's wire key set must stay fixed -- both embedding CRDs "+
			"(SelectedWorkflow, WorkflowRef) rely on this single type for their schema, "+
			"so a key added/removed here changes both simultaneously by construction")

		var roundTripped sharedtypes.WorkflowSnapshot
		Expect(json.Unmarshal(raw, &roundTripped)).To(Succeed())
		Expect(roundTripped.ActionType).To(Equal("ScaleReplicas"))
		Expect(roundTripped.WorkflowName).To(Equal("oom-recovery"))
		Expect(roundTripped.WorkflowID).To(Equal("wf-oom-recovery"))
	})

	It("UT-SHARED-1661-654-002: omits optional fields from the wire payload when unset, but never the four Required fields", func() {
		snap := sharedtypes.WorkflowSnapshot{
			WorkflowID:      "wf-minimal",
			WorkflowName:    "minimal",
			ActionType:      "RestartPod",
			Version:         "v1.0.0",
			ExecutionBundle: "quay.io/kubernaut/minimal:v1",
			ExecutionEngine: "tekton",
		}

		raw, err := json.Marshal(snap)
		Expect(err).NotTo(HaveOccurred())

		var asMap map[string]interface{}
		Expect(json.Unmarshal(raw, &asMap)).To(Succeed())

		By("always including the four catalog-authoritative Required fields")
		Expect(asMap).To(HaveKey("workflowId"))
		Expect(asMap).To(HaveKey("workflowName"))
		Expect(asMap).To(HaveKey("actionType"))
		Expect(asMap).To(HaveKey("executionEngine"))

		By("omitting unset optional fields (omitempty)")
		Expect(asMap).NotTo(HaveKey("executionBundleDigest"))
		Expect(asMap).NotTo(HaveKey("engineConfig"))
		Expect(asMap).NotTo(HaveKey("serviceAccountName"))
		Expect(asMap).NotTo(HaveKey("dependencies"))
		Expect(asMap).NotTo(HaveKey("resources"))
	})
})
