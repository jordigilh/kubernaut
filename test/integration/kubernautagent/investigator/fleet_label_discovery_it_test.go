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
	"context"
	"encoding/json"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	meta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fleetLabelCoverageTool is a deterministic remote-cluster stand-in for the
// resources_get/resources_list overlay tools. It intentionally keys responses
// by the requested kind so a test cannot pass by returning one canned object
// for every remote lookup.
type fleetLabelCoverageTool struct {
	name         string
	root         string
	getByKind    map[string]string
	listByKind   map[string]string
	requestedK8s []string
}

func (t *fleetLabelCoverageTool) Name() string                { return t.name }
func (t *fleetLabelCoverageTool) Description() string         { return "remote label coverage " + t.name }
func (t *fleetLabelCoverageTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (t *fleetLabelCoverageTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var request struct {
		Kind string `json:"kind"`
	}
	Expect(json.Unmarshal(args, &request)).To(Succeed())
	t.requestedK8s = append(t.requestedK8s, request.Kind)

	if t.name == "resources_list" {
		if response, ok := t.listByKind[request.Kind]; ok {
			return response, nil
		}
		return `{"items":[]}`, nil
	}
	if response, ok := t.getByKind[request.Kind]; ok {
		return response, nil
	}
	if request.Kind == "Namespace" {
		return `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"remote-ns"}}`, nil
	}
	return t.root, nil
}

type fleetLabelCoverageCase struct {
	name        string
	targetKind  string
	apiVersion  string
	root        string
	getByKind   map[string]string
	listByKind  map[string]string
	expectedKey string
	expectedVal string
}

func fleetLabelRoot(kind, apiVersion string, labels, annotations, podAnnotations map[string]string) string {
	metadata := map[string]interface{}{
		"name":      "remote-target",
		"namespace": "remote-ns",
	}
	if len(labels) > 0 {
		metadata["labels"] = labels
	}
	if len(annotations) > 0 {
		metadata["annotations"] = annotations
	}
	object := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   metadata,
	}
	if kind == "Deployment" || kind == "StatefulSet" {
		object["spec"] = map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels":      map[string]string{"app": "remote-target"},
					"annotations": podAnnotations,
				},
			},
		}
	}
	if kind == "VirtualMachine" {
		object["spec"] = map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{},
			},
		}
	}
	data, err := json.Marshal(object)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func fleetLabelCoverageMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "autoscaling", Version: "v2"},
		{Group: "policy", Version: "v1"},
		{Group: "networking.k8s.io", Version: "v1"},
		{Group: "kubevirt.io", Version: "v1"},
		{Group: "storage.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ResourceQuota"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "StorageClass"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"}, meta.RESTScopeNamespace)
	return mapper
}

func fleetLabelCoverageCases() []fleetLabelCoverageCase {
	deployment := func(labels, annotations, podAnnotations map[string]string) string {
		return fleetLabelRoot("Deployment", "apps/v1", labels, annotations, podAnnotations)
	}
	statefulSet := fleetLabelRoot("StatefulSet", "apps/v1", nil, nil, nil)
	vm := func(liveMigrate bool) string {
		root := fleetLabelRoot("VirtualMachine", "kubevirt.io/v1", nil, nil, nil)
		if !liveMigrate {
			return root
		}
		return `{"apiVersion":"kubevirt.io/v1","kind":"VirtualMachine","metadata":{"name":"remote-target","namespace":"remote-ns"},"spec":{"template":{"spec":{"evictionStrategy":"LiveMigrate"}}}}`
	}
	pvcList := func(annotation bool) string {
		annotations := ""
		if annotation {
			annotations = `,"annotations":{"cdi.kubevirt.io/storage.import.endpoint":"https://example.invalid/disk.qcow2"}`
		}
		return `{"apiVersion":"v1","kind":"PersistentVolumeClaimList","items":[{"metadata":{"name":"remote-disk","namespace":"remote-ns"` + annotations + `},"spec":{"storageClassName":"remote-sc"}}]}`
	}
	storageClass := `{"apiVersion":"storage.k8s.io/v1","kind":"StorageClass","metadata":{"name":"remote-sc"},"provisioner":"topolvm.io"}`

	return []fleetLabelCoverageCase{
		{
			name: "gitOpsManaged", targetKind: "Deployment", apiVersion: "apps/v1",
			root:        deployment(nil, map[string]string{"argocd.argoproj.io/tracking-id": "remote-app:apps/Deployment:remote-ns/remote-target"}, nil),
			expectedKey: "gitOpsManaged", expectedVal: "true",
		},
		{
			name: "gitOpsTool", targetKind: "Deployment", apiVersion: "apps/v1",
			root:        deployment(nil, map[string]string{"argocd.argoproj.io/tracking-id": "remote-app:apps/Deployment:remote-ns/remote-target"}, nil),
			expectedKey: "gitOpsTool", expectedVal: "argocd",
		},
		{
			name: "helmManaged", targetKind: "Deployment", apiVersion: "apps/v1",
			root:        deployment(map[string]string{"app.kubernetes.io/managed-by": "Helm"}, nil, nil),
			expectedKey: "helmManaged", expectedVal: "true",
		},
		{
			name: "stateful", targetKind: "StatefulSet", apiVersion: "apps/v1",
			root:        statefulSet,
			expectedKey: "stateful", expectedVal: "true",
		},
		{
			name: "serviceMesh", targetKind: "Deployment", apiVersion: "apps/v1",
			root:        deployment(nil, nil, map[string]string{"sidecar.istio.io/status": "{}"}),
			expectedKey: "serviceMesh", expectedVal: "istio",
		},
		{
			name: "hpaEnabled", targetKind: "Deployment", apiVersion: "apps/v1",
			root: deployment(nil, nil, nil),
			listByKind: map[string]string{
				"HorizontalPodAutoscaler": `{"apiVersion":"autoscaling/v2","kind":"HorizontalPodAutoscalerList","items":[{"spec":{"scaleTargetRef":{"kind":"Deployment","name":"remote-target"}}}]}`,
			},
			expectedKey: "hpaEnabled", expectedVal: "true",
		},
		{
			name: "pdbProtected", targetKind: "Deployment", apiVersion: "apps/v1",
			root: deployment(nil, nil, nil),
			listByKind: map[string]string{
				"PodDisruptionBudget": `{"apiVersion":"policy/v1","kind":"PodDisruptionBudgetList","items":[{"spec":{"selector":{"matchLabels":{"app":"remote-target"}}}}]}`,
			},
			expectedKey: "pdbProtected", expectedVal: "true",
		},
		{
			name: "networkIsolated", targetKind: "Deployment", apiVersion: "apps/v1",
			root: deployment(nil, nil, nil),
			listByKind: map[string]string{
				"NetworkPolicy": `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicyList","items":[{"metadata":{"name":"remote-isolation"}}]}`,
			},
			expectedKey: "networkIsolated", expectedVal: "true",
		},
		{
			name: "resourceQuotaConstrained", targetKind: "Deployment", apiVersion: "apps/v1",
			root: deployment(nil, nil, nil),
			listByKind: map[string]string{
				"ResourceQuota": `{"apiVersion":"v1","kind":"ResourceQuotaList","items":[{"metadata":{"name":"remote-quota"},"status":{"hard":{"pods":"10"},"used":{"pods":"1"}}}]}`,
			},
			expectedKey: "resourceQuotaConstrained", expectedVal: "true",
		},
		{
			name: "virtualMachine", targetKind: "VirtualMachine", apiVersion: "kubevirt.io/v1",
			root:        vm(false),
			expectedKey: "virtualMachine", expectedVal: "true",
		},
		{
			name: "liveMigratable", targetKind: "VirtualMachine", apiVersion: "kubevirt.io/v1",
			root:        vm(true),
			expectedKey: "liveMigratable", expectedVal: "true",
		},
		{
			name: "cdiManaged", targetKind: "VirtualMachine", apiVersion: "kubevirt.io/v1",
			root: vm(false), listByKind: map[string]string{
				"PersistentVolumeClaim": pvcList(true),
			},
			expectedKey: "cdiManaged", expectedVal: "true",
		},
		{
			name: "storageBackend", targetKind: "VirtualMachine", apiVersion: "kubevirt.io/v1",
			root: vm(false), getByKind: map[string]string{
				"StorageClass": storageClass,
			}, listByKind: map[string]string{
				"PersistentVolumeClaim": pvcList(false),
			},
			expectedKey: "storageBackend", expectedVal: "lvms",
		},
	}
}

// IT-KA-FLEET-LABELS [AC-4, AC-6, BR-AI-056, Issue #2417] proves every
// authoritative detection category through the fleet-target post-RCA
// workflow-discovery path. Each entry enables one category's remote fixture so
// a missing overlay lookup or category-specific remote request cannot be hidden
// by a broad fixture that happens to make several labels true at once.
var _ = DescribeTable("IT-KA-FLEET-LABELS: remote label detection through workflow discovery", func(tc fleetLabelCoverageCase) {
	auditStore := newCapturingAuditStore(suiteAuditStore)
	getTool := &fleetLabelCoverageTool{
		name: "resources_get", root: tc.root, getByKind: tc.getByKind,
		listByKind: tc.listByKind,
	}
	listTool := &fleetLabelCoverageTool{
		name: "resources_list", root: tc.root, getByKind: tc.getByKind,
		listByKind: tc.listByKind,
	}
	spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{
		"resources_get":  getTool,
		"resources_list": listTool,
	}}
	hubK8s := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
		{Kind: "Deployment", Name: "hub-should-not-appear", Namespace: "remote-ns"},
	}}
	hubDetector := enrichment.NewLabelDetector(nil, fleetLabelCoverageMapper(), logr.Discard())
	enricher := enrichment.NewEnricher(hubK8s, suiteDSAdapter, auditStore, logr.Discard()).
		WithK8sResolver(func(ctx context.Context) enrichment.K8sClient {
			return custom.ResolveK8sClient(ctx, hubK8s, logr.Discard())
		}).
		WithLabelDetectorResolver(func(ctx context.Context) *enrichment.LabelDetector {
			return custom.ResolveLabelDetector(ctx, hubDetector, fleetLabelCoverageMapper(), logr.Discard())
		})
	builder, err := prompt.NewBuilder()
	Expect(err).NotTo(HaveOccurred())
	mockClient := &mockLLMClient{responses: []llm.ChatResponse{
		wfToolResp(`{"selected_workflow":{"workflow_id":"fleet-label-coverage","confidence":0.9},"confidence":0.9}`),
	}}
	inv := investigator.New(investigator.Config{
		Client: mockClient, Builder: builder, ResultParser: parser.NewResultParser(),
		Enricher: enricher, AuditStore: auditStore, Logger: logr.Discard(), MaxTurns: 15,
		PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
		FleetOverlayResolver: spy,
	})

	result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), katypes.SignalContext{
		Name: "remote-target", Namespace: "remote-ns", ResourceKind: tc.targetKind,
		ResourceName: "remote-target", ResourceAPIVersion: tc.apiVersion,
		ClusterID: "remote-east", RemediationID: "rem-fleet-labels-" + tc.name,
	}, &katypes.InvestigationResult{
		RCASummary: "remote label coverage",
		RemediationTarget: katypes.RemediationTarget{
			Kind: tc.targetKind, Name: "remote-target", Namespace: "remote-ns", APIVersion: tc.apiVersion,
		},
	}, nil, "corr-fleet-labels-"+tc.name)
	Expect(err).NotTo(HaveOccurred())
	Expect(result).NotTo(BeNil())
	Expect(result.WorkflowID).To(Equal("fleet-label-coverage"))
	Expect(spy.calls).To(ConsistOf("remote-east"))
	Expect(result.DetectedLabels).NotTo(BeNil())
	if tc.expectedVal == "true" {
		Expect(result.DetectedLabels[tc.expectedKey]).To(BeTrue(),
			"remote fleet detection for %s must survive post-RCA workflow discovery", tc.name)
	} else {
		Expect(result.DetectedLabels[tc.expectedKey]).To(Equal(tc.expectedVal),
			"remote fleet detection for %s must survive post-RCA workflow discovery", tc.name)
	}
	failed, ok := result.DetectedLabels["failedDetections"].([]string)
	if ok {
		Expect(failed).To(BeEmpty(),
			"remote fleet detection for %s must not silently degrade category requests", tc.name)
	}
},
	Entry("gitOpsManaged", fleetLabelCoverageCases()[0]),
	Entry("gitOpsTool", fleetLabelCoverageCases()[1]),
	Entry("helmManaged", fleetLabelCoverageCases()[2]),
	Entry("stateful", fleetLabelCoverageCases()[3]),
	Entry("serviceMesh", fleetLabelCoverageCases()[4]),
	Entry("hpaEnabled", fleetLabelCoverageCases()[5]),
	Entry("pdbProtected", fleetLabelCoverageCases()[6]),
	Entry("networkIsolated", fleetLabelCoverageCases()[7]),
	Entry("resourceQuotaConstrained", fleetLabelCoverageCases()[8]),
	Entry("virtualMachine", fleetLabelCoverageCases()[9]),
	Entry("liveMigratable", fleetLabelCoverageCases()[10]),
	Entry("cdiManaged", fleetLabelCoverageCases()[11]),
	Entry("storageBackend", fleetLabelCoverageCases()[12]),
)
