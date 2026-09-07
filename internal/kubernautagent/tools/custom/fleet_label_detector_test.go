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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
)

type fleetLabelTool struct {
	list bool
}

func (t *fleetLabelTool) Name() string {
	if t.list {
		return "resources_list"
	}
	return "resources_get"
}
func (t *fleetLabelTool) Description() string         { return "fleet label detector test tool" }
func (t *fleetLabelTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (t *fleetLabelTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var request map[string]string
	_ = json.Unmarshal(args, &request)
	if t.list {
		switch request["kind"] {
		case "HorizontalPodAutoscaler":
			return `{"items":[{"kind":"HorizontalPodAutoscaler","spec":{"scaleTargetRef":{"kind":"VirtualMachine","name":"vm-1"}}}]}`, nil
		case "PodDisruptionBudget":
			return `{"items":[{"kind":"PodDisruptionBudget","spec":{"selector":{"matchLabels":{"app":"vm"}}}}]}`, nil
		case "NetworkPolicy":
			return `{"items":[{"kind":"NetworkPolicy","metadata":{"name":"isolate"}}]}`, nil
		case "ResourceQuota":
			return `{"items":[{"kind":"ResourceQuota","status":{"hard":{"pods":"10"},"used":{"pods":"1"}}}]}`, nil
		case "PersistentVolumeClaim":
			return `{"items":[{"kind":"PersistentVolumeClaim","metadata":{"annotations":{"cdi.kubevirt.io/storage.import.endpoint":"https://example.invalid/disk"}},"spec":{"storageClassName":"remote-sc"}}]}`, nil
		default:
			return `{"items":[]}`, nil
		}
	}
	if request["kind"] == "StorageClass" {
		return `{"kind":"StorageClass","metadata":{"name":"remote-sc"},"provisioner":"topolvm.io"}`, nil
	}
	return `{"kind":"VirtualMachine","metadata":{"name":"vm-1","namespace":"remote-ns","labels":{"app":"vm"}},"spec":{"template":{"spec":{"evictionStrategy":"LiveMigrate"}}}}`, nil
}

func fleetLabelMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"}, {Group: "autoscaling", Version: "v2"},
		{Group: "policy", Version: "v1"}, {Group: "networking.k8s.io", Version: "v1"},
		{Group: "kubevirt.io", Version: "v1"}, {Group: "storage.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ResourceQuota"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"}, meta.RESTScopeRoot)
	return mapper
}

var _ = Describe("IT-KA-2345: fleet LabelDetector", Label("it", "fleet", "2345"), func() {
	It("detects list-backed categories and storage details from the remote cluster", func() {
		getTool := &fleetLabelTool{}
		listTool := &fleetLabelTool{list: true}
		detector := custom.NewOverlayLabelDetector(getTool, listTool, fleetLabelMapper(), logr.Discard())

		labels, quotas, err := detector.DetectLabels(context.Background(), "VirtualMachine", "vm-1", "remote-ns", []enrichment.OwnerChainEntry{{Kind: "VirtualMachine", Name: "vm-1", Namespace: "remote-ns"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(labels.HPAEnabled).To(BeTrue())
		Expect(labels.PDBProtected).To(BeTrue())
		Expect(labels.NetworkIsolated).To(BeTrue())
		Expect(labels.ResourceQuotaConstrained).To(BeTrue())
		Expect(labels.CDIManaged).To(BeTrue())
		Expect(labels.StorageBackend).To(Equal("lvms"))
		Expect(quotas).To(HaveKey("pods"))
	})
})
