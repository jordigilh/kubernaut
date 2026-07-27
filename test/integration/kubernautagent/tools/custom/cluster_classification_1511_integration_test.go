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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// IT-KA-1511-001: the `cluster` discovery param reaches KA's own workflow
// catalog cache through the production dispatch path (BR-FLEET-003, #1511,
// AU-3). #1677 Phase 2e (DD-WORKFLOW-019): the discovery tools no longer
// round-trip through DataStorage -- filtering (including cluster-based
// filtering, ported in Phase 2b's cache_filter.go) now happens against KA's
// own informer-backed cache. This test proves the KA -> workflowcatalog.
// Catalog dispatch succeeds end-to-end with the ClusterClassification signal
// field present, i.e. the wiring itself is correct and introduces no
// regression for non-fleet callers.
var _ = Describe("IT-KA-1511-001: cluster classification param reaches KA's workflow catalog (BR-FLEET-003)", Label("integration", "fleet"), func() {

	var reg *registry.Registry

	BeforeEach(func() {
		Expect(wfCatalog).NotTo(BeNil(), "workflow catalog must be initialized by SynchronizedBeforeSuite")

		reg = registry.New()
		allTools := custom.NewAllTools(wfCatalog, nil, logr.Discard())
		for _, t := range allTools {
			reg.Register(t)
		}
	})

	clusterToolCtx := func(cluster string) context.Context {
		return katypes.WithSignalContext(context.Background(), katypes.SignalContext{
			Severity:              "critical",
			ResourceKind:          "Deployment",
			Environment:           "production",
			Priority:              "P0",
			ClusterClassification: cluster,
		})
	}

	It("IT-KA-1511-001a: list_available_actions succeeds against real DS when ClusterClassification is set", func() {
		result, err := reg.Execute(clusterToolCtx("production"), "list_available_actions",
			json.RawMessage(`{}`))
		Expect(err).NotTo(HaveOccurred(), "the cluster param must not break the real DS discovery call")
		Expect(result).NotTo(BeEmpty())
		Expect(result).To(ContainSubstring("actionTypes"))
	})

	It("IT-KA-1511-001b: list_workflows succeeds against real DS when ClusterClassification is set", func() {
		result, err := reg.Execute(clusterToolCtx("production"), "list_workflows",
			json.RawMessage(`{"action_type":"IncreaseMemoryLimits"}`))
		Expect(err).NotTo(HaveOccurred(), "the cluster param must not break the real DS discovery call")
		Expect(result).NotTo(BeEmpty())
		Expect(result).To(ContainSubstring("workflows"))
	})

	It("IT-KA-1511-001c: no ClusterClassification (non-fleet) is a zero behavioral change against real DS", func() {
		result, err := reg.Execute(clusterToolCtx(""), "list_available_actions",
			json.RawMessage(`{}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeEmpty())
		Expect(result).To(ContainSubstring("actionTypes"))
	})
})
