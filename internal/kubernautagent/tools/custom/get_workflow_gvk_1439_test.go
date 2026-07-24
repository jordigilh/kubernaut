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

package custom_test

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Issue #1439: get_workflow component filter must use GVK format", func() {

	Describe("UT-KA-GW-001: get_workflow sends ComponentGVK when apiVersion is set", func() {
		It("should send v1/ConfigMap as component, not configmap", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			ctx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "ConfigMap",
				ResourceAPIVersion: "v1",
				ResourceName:       "worker-config",
				Namespace:          "demo-storefront",
				Environment:        "staging",
				Priority:           "P1",
				RemediationID:      "rr-gvk-test-001",
			})

			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.getWorkflowFilters).NotTo(BeNil())
			Expect(fake.getWorkflowFilters.Component).To(Equal("v1/ConfigMap"),
				"get_workflow must send GVK format (apiVersion/Kind), not bare lowercase kind")
		})
	})

	Describe("UT-KA-GW-002: get_workflow falls back to lowercase Kind when GVK is empty", func() {
		It("should send configmap as component when ResourceAPIVersion is empty", func() {
			fake := &fakeWorkflowDS{}
			allTools := newTestTools(fake)
			getWorkflow := allTools[2]

			ctx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
				Severity:           "critical",
				ResourceKind:       "ConfigMap",
				ResourceAPIVersion: "",
				ResourceName:       "worker-config",
				Namespace:          "demo-storefront",
				Environment:        "staging",
				Priority:           "P1",
				RemediationID:      "rr-gvk-test-002",
			})

			validUUID := uuid.New().String()
			args := json.RawMessage(`{"workflow_id":"` + validUUID + `"}`)

			_, err := getWorkflow.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			Expect(fake.getWorkflowFilters).NotTo(BeNil())
			Expect(fake.getWorkflowFilters.Component).To(Equal("configmap"),
				"get_workflow should fall back to lowercase Kind when ResourceAPIVersion is empty")
		})
	})
})
