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

package conversation_test

import (
	"context"
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
)

var _ = Describe("UT-CS-592-034: DynamicRARReader — RAR context extraction", Label("unit", "kubernautagent", "conversation", "cs-592"), func() {

	var (
		ctx    context.Context
		logger *slog.Logger
		gvr    schema.GroupVersionResource
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
		gvr = schema.GroupVersionResource{
			Group:    "kubernaut.ai",
			Version:  "v1alpha1",
			Resource: "remediationapprovalrequests",
		}
	})

	// BR-CONV-001: Conversation sessions must include investigation context.
	It("UT-CS-592-034: returns investigation summary, reason, and confidence from a valid RAR", func() {
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "test-rar",
					"namespace": "test-ns",
				},
				"spec": map[string]interface{}{
					"investigationSummary": "- Signal Name: KubePodCrashLooping\nPod is crash-looping due to config error",
					"reason":               "Missing configuration file",
					"confidence":           0.85,
				},
			},
		}

		scheme := runtime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{gvr: "RemediationApprovalRequestList"},
			rar,
		)

		reader := conversation.NewDynamicRARReader(dynClient, logger)
		result, err := reader.GetRARContext(ctx, "test-ns", "test-rar")

		Expect(err).ToNot(HaveOccurred(), "GET on existing RAR should succeed")
		Expect(result.InvestigationSummary).To(ContainSubstring("Signal Name: KubePodCrashLooping"),
			"InvestigationSummary should contain the signal name line from the RAR spec")
		Expect(result.Reason).To(Equal("Missing configuration file"),
			"Reason should match the RAR spec.reason field")
		Expect(result.Confidence).To(BeNumerically("~", 0.85, 0.001),
			"Confidence should match the RAR spec.confidence value")
	})

	It("UT-CS-592-035: returns error when RAR does not exist", func() {
		scheme := runtime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{gvr: "RemediationApprovalRequestList"},
		)

		reader := conversation.NewDynamicRARReader(dynClient, logger)
		result, err := reader.GetRARContext(ctx, "missing-ns", "missing-rar")

		Expect(err).To(HaveOccurred(), "GET on nonexistent RAR should return error")
		Expect(err.Error()).To(ContainSubstring("missing-rar"),
			"Error message should reference the RAR name for debugging")
		Expect(result).To(BeNil(), "Result should be nil when GET fails")
	})

	It("UT-CS-592-036: returns empty fields when RAR has no investigation summary", func() {
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "empty-rar",
					"namespace": "test-ns",
				},
				"spec": map[string]interface{}{
					"confidence": 0.0,
				},
			},
		}

		scheme := runtime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{gvr: "RemediationApprovalRequestList"},
			rar,
		)

		reader := conversation.NewDynamicRARReader(dynClient, logger)
		result, err := reader.GetRARContext(ctx, "test-ns", "empty-rar")

		Expect(err).ToNot(HaveOccurred(), "GET on RAR with missing optional fields should succeed")
		Expect(result.InvestigationSummary).To(BeEmpty(),
			"InvestigationSummary should be empty when field is absent in the RAR spec")
		Expect(result.Reason).To(BeEmpty(),
			"Reason should be empty when field is absent in the RAR spec")
		Expect(result.Confidence).To(BeNumerically("==", 0.0),
			"Confidence should be zero when field is absent in the RAR spec")
	})
})
