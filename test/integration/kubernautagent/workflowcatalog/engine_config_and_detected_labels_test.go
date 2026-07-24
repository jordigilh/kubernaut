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

package workflowcatalog_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// Issue #1677 Phase 2g (DD-WORKFLOW-019): relocates the DataStorage-side
// business-logic IT coverage this replaces:
//   - EngineConfig discriminator pattern + float parameters
//     (test/integration/datastorage/workflow_engine_config_integration_test.go,
//     BR-WE-016, BR-WORKFLOW-005)
//   - DetectedLabels JSONB round-trip fidelity
//     (test/integration/datastorage/workflow_detected_labels_test.go, ADR-043 v1.3)
//   - CNV-specific detected-label scoring/ranking
//     (test/integration/datastorage/workflow_detected_labels_cnv_test.go, #1378)
//
// The underlying conversion logic (crdWorkflowToModel/crdDetectedLabelsToModel/
// wrapCRDParameters in cache_convert.go) and scoring/ranking logic
// (cache_filter.go) were ported verbatim from pkg/datastorage/repository/workflow
// in Phase 2b; this file proves that port against a real envtest API server,
// via CRD-native construction (rw.Spec.EngineConfig/DetectedLabels as raw JSON)
// rather than DS's retired admission-simulation seeding helper.
//
// Business Requirements: BR-WE-016, BR-WORKFLOW-005, BR-WORKFLOW-018,
// BR-WORKFLOW-004.
var _ = Describe("IT-KA-1677 EngineConfig and DetectedLabels catalog round-trip", Label("integration", "kubernautagent", "workflow-catalog"), func() {

	var (
		wfCache     *workflowcatalog.Cache
		cacheCancel func()
		catalog     *workflowcatalog.Catalog
	)

	uniqueName := func(prefix string) string {
		return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), GinkgoParallelProcess())
	}
	uniquePascalName := func(prefix string) string {
		return fmt.Sprintf("%s%d%d", prefix, time.Now().UnixNano(), GinkgoParallelProcess())
	}

	BeforeEach(func() {
		scheme, schemeErr := workflowcatalog.NewScheme()
		Expect(schemeErr).ToNot(HaveOccurred())

		var err error
		wfCache, cacheCancel, err = workflowcatalog.NewInformerCache(k8sConfig, scheme, logger)
		Expect(err).ToNot(HaveOccurred(), "workflow catalog cache should build and sync against the shared envtest")
		catalog = workflowcatalog.NewCatalog(wfCache, logger)
	})

	AfterEach(func() {
		if cacheCancel != nil {
			cacheCancel()
		}
	})

	// unwrapParameters extracts the flat parameter list from the
	// {"schema":{"parameters":[...]}} envelope wrapCRDParameters
	// (cache_convert.go) wraps spec.parameters[] in.
	unwrapParameters := func(raw *json.RawMessage) []models.WorkflowParameter {
		var wrapper struct {
			Schema struct {
				Parameters []models.WorkflowParameter `json:"parameters"`
			} `json:"schema"`
		}
		Expect(json.Unmarshal(*raw, &wrapper)).To(Succeed())
		return wrapper.Schema.Parameters
	}

	// jsonRaw marshals v into an *apiextensionsv1.JSON, the wire shape
	// rw.Spec.Execution.EngineConfig/rw.Spec.DetectedLabels use.
	jsonRaw := func(v interface{}) *apiextensionsv1.JSON {
		raw, err := json.Marshal(v)
		Expect(err).ToNot(HaveOccurred())
		return &apiextensionsv1.JSON{Raw: raw}
	}

	// wfFixture is the field set varied below; zero-value fields keep
	// sensible single-value defaults (see newWF).
	type wfFixture struct {
		Engine         string
		EngineConfig   map[string]interface{}
		Parameters     []rwv1alpha1.RemediationWorkflowParameter
		DetectedLabels *models.DetectedLabels
		Severity       []string
		Component      []string
		Environment    []string
		CustomLabels   map[string]string
	}

	newWF := func(name, actionType string, f wfFixture) *rwv1alpha1.RemediationWorkflow {
		engine := f.Engine
		if engine == "" {
			engine = "tekton"
		}
		severity := f.Severity
		if len(severity) == 0 {
			severity = []string{"critical"}
		}
		component := f.Component
		if len(component) == 0 {
			component = []string{"v1/Pod"}
		}
		environment := f.Environment
		if len(environment) == 0 {
			environment = []string{"production"}
		}

		rw := &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version:    "1.0.0",
				ActionType: actionType,
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "IT-KA-1677 EngineConfig/DetectedLabels test fixture: " + name,
					WhenToUse: "For workflow catalog EngineConfig/DetectedLabels integration testing",
				},
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    severity,
					Environment: environment,
					Component:   component,
					Priority:    "P1",
				},
				CustomLabels: f.CustomLabels,
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine: engine,
					Bundle: testutil.ValidBundleRef,
				},
				Parameters: f.Parameters,
			},
		}
		if len(rw.Spec.Parameters) == 0 {
			rw.Spec.Parameters = []rwv1alpha1.RemediationWorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
			}
		}
		if f.EngineConfig != nil {
			rw.Spec.Execution.EngineConfig = jsonRaw(f.EngineConfig)
		}
		if f.DetectedLabels != nil {
			rw.Spec.DetectedLabels = jsonRaw(*f.DetectedLabels)
		}
		return rw
	}

	// createWF creates rw, defers its cleanup, and blocks until this spec's
	// own cache has observed it (by content, since these fixtures have no
	// stable action-type-scoped assertion the way discovery_edge_cases_test.go's
	// createRW does).
	createWF := func(rw *rwv1alpha1.RemediationWorkflow) {
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rw) })
		Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
			return wfCache.GetWorkflow(ctx, rw.Name)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(), "cache must observe %s before the spec asserts on it", rw.Name)
	}

	getByID := func(workflowID string) *models.RemediationWorkflow {
		var got *models.RemediationWorkflow
		Eventually(func() (*models.RemediationWorkflow, error) {
			var err error
			got, err = catalog.GetByID(ctx, workflowID)
			return got, err
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil())
		return got
	}

	// ========================================
	// EngineConfig discriminator pattern (BR-WE-016) + float params (BR-WORKFLOW-005)
	// (was IT-WE-016-*/IT-WF-005-*)
	// ========================================
	Describe("EngineConfig discriminator pattern (BR-WE-016)", func() {
		It("IT-KA-1677-WE016-001: stores and retrieves engineConfig in the catalog", func() {
			at := uniquePascalName("EngineConfigAction")
			name := uniqueName("wf-engineconfig")
			ansibleConfig := map[string]interface{}{
				"playbookPath":    "playbooks/restart.yml",
				"jobTemplateName": "restart-pod",
				"inventoryName":   "production",
			}
			workflowID := uniqueName("wfid-engineconfig")
			rw := newWF(name, at, wfFixture{Engine: "ansible", EngineConfig: ansibleConfig})
			createWF(rw)
			rw.Status.WorkflowID = workflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

			retrieved := getByID(workflowID)
			Expect(retrieved.ExecutionEngine).To(Equal(models.ExecutionEngineAnsible))
			Expect(retrieved.EngineConfig).ToNot(BeNil(), "EngineConfig should be preserved after roundtrip")

			var parsedConfig map[string]interface{}
			Expect(json.Unmarshal(*retrieved.EngineConfig, &parsedConfig)).To(Succeed())
			Expect(parsedConfig["playbookPath"]).To(Equal("playbooks/restart.yml"))
			Expect(parsedConfig["jobTemplateName"]).To(Equal("restart-pod"))
			Expect(parsedConfig["inventoryName"]).To(Equal("production"))
		})

		It("IT-KA-1677-WE016-001b: stores a tekton workflow without engineConfig as nil", func() {
			at := uniquePascalName("NoEngineConfigAction")
			name := uniqueName("wf-no-engineconfig")
			workflowID := uniqueName("wfid-no-engineconfig")
			rw := newWF(name, at, wfFixture{Engine: "tekton"})
			createWF(rw)
			rw.Status.WorkflowID = workflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

			retrieved := getByID(workflowID)
			Expect(retrieved.ExecutionEngine).To(Equal(models.ExecutionEngineTekton))
			Expect(retrieved.EngineConfig).To(BeNil(), "Tekton workflows should have nil engineConfig")
		})

		It("IT-KA-1677-WE016-002: returns engineConfig in List results", func() {
			at := uniquePascalName("EngineConfigListAction")
			name := uniqueName("wf-engineconfig-list")
			ansibleConfig := map[string]interface{}{
				"playbookPath":    "playbooks/scale-down.yml",
				"jobTemplateName": "scale-down-svc",
			}
			createWF(newWF(name, at, wfFixture{Engine: "ansible", EngineConfig: ansibleConfig}))

			var results []models.RemediationWorkflow
			Eventually(func() (int, error) {
				var total int
				var err error
				results, total, err = catalog.List(ctx, &models.WorkflowSearchFilters{WorkflowName: name}, 50, 0)
				return total, err
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))
			Expect(results).To(HaveLen(1))
			Expect(results[0].EngineConfig).ToNot(BeNil(), "EngineConfig must be present in List results")

			var parsedConfig map[string]interface{}
			Expect(json.Unmarshal(*results[0].EngineConfig, &parsedConfig)).To(Succeed())
			Expect(parsedConfig["playbookPath"]).To(Equal("playbooks/scale-down.yml"))
		})
	})

	Describe("Float parameter persistence (BR-WORKFLOW-005)", func() {
		It("IT-KA-1677-WF005-001: stores and retrieves a workflow with float parameters", func() {
			at := uniquePascalName("FloatParamAction")
			name := uniqueName("wf-float-param")
			workflowID := uniqueName("wfid-float-param")
			minVal := 0.1
			maxVal := 99.9
			params := []rwv1alpha1.RemediationWorkflowParameter{
				{
					Name:        "cpu_threshold",
					Type:        "float",
					Description: "CPU threshold percentage",
					Required:    true,
					Minimum:     &minVal,
					Maximum:     &maxVal,
				},
				{
					Name:        "timeout",
					Type:        "integer",
					Description: "Timeout in seconds",
					Required:    false,
				},
			}
			rw := newWF(name, at, wfFixture{Engine: "tekton", Parameters: params})
			createWF(rw)
			rw.Status.WorkflowID = workflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

			retrieved := getByID(workflowID)
			Expect(retrieved.Parameters).ToNot(BeNil())
			retrievedParams := unwrapParameters(retrieved.Parameters)
			Expect(retrievedParams).To(HaveLen(2))

			cpuParam := retrievedParams[0]
			Expect(cpuParam.Name).To(Equal("cpu_threshold"))
			Expect(cpuParam.Type).To(Equal("float"))
			Expect(*cpuParam.Minimum).To(BeNumerically("~", 0.1, 0.001))
			Expect(*cpuParam.Maximum).To(BeNumerically("~", 99.9, 0.001))
		})
	})

	// ========================================
	// DetectedLabels JSONB round-trip fidelity (ADR-043 v1.3) (was IT-DS-043-*)
	// ========================================
	Describe("DetectedLabels round-trip fidelity (ADR-043 v1.3)", func() {
		It("IT-KA-1677-043-001: a workflow with detectedLabels is stored accurately in the catalog", func() {
			at := uniquePascalName("DetectedLabelsAllAction")
			name := uniqueName("wf-dl-all-fields")
			workflowID := uniqueName("wfid-dl-all")
			dl := models.DetectedLabels{
				GitOpsManaged:   true,
				GitOpsTool:      "argocd",
				PDBProtected:    true,
				HPAEnabled:      true,
				Stateful:        true,
				HelmManaged:     true,
				NetworkIsolated: true,
				ServiceMesh:     "istio",
			}
			rw := newWF(name, at, wfFixture{DetectedLabels: &dl})
			createWF(rw)
			rw.Status.WorkflowID = workflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

			retrieved := getByID(workflowID)
			Expect(retrieved.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("argocd"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeTrue())
			Expect(retrieved.DetectedLabels.HPAEnabled).To(BeTrue())
			Expect(retrieved.DetectedLabels.Stateful).To(BeTrue())
			Expect(retrieved.DetectedLabels.HelmManaged).To(BeTrue())
			Expect(retrieved.DetectedLabels.NetworkIsolated).To(BeTrue())
			Expect(retrieved.DetectedLabels.ServiceMesh).To(Equal("istio"))
		})

		It("IT-KA-1677-043-002: a retrieved workflow returns exactly the detectedLabels registered, with unset fields defaulted", func() {
			at := uniquePascalName("DetectedLabelsPartialAction")
			name := uniqueName("wf-dl-partial-fields")
			workflowID := uniqueName("wfid-dl-partial")
			dl := models.DetectedLabels{HPAEnabled: true, GitOpsTool: "*"}
			rw := newWF(name, at, wfFixture{DetectedLabels: &dl})
			createWF(rw)
			rw.Status.WorkflowID = workflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

			retrieved := getByID(workflowID)
			Expect(retrieved.DetectedLabels.HPAEnabled).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("*"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeFalse(), "unset boolean should be false after round-trip")
			Expect(retrieved.DetectedLabels.ServiceMesh).To(BeEmpty(), "unset string should be empty after round-trip")
		})

		It("IT-KA-1677-043-003: a workflow without detectedLabels has empty DetectedLabels in the catalog", func() {
			at := uniquePascalName("DetectedLabelsNoneAction")
			name := uniqueName("wf-dl-none")
			workflowID := uniqueName("wfid-dl-none")
			rw := newWF(name, at, wfFixture{})
			createWF(rw)
			rw.Status.WorkflowID = workflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

			retrieved := getByID(workflowID)
			Expect(retrieved.DetectedLabels.IsEmpty()).To(BeTrue(), "empty DetectedLabels should round-trip as empty, not null")
		})

		It("IT-KA-1677-043-006: all fields are preserved alongside detectedLabels", func() {
			at := uniquePascalName("DetectedLabelsFullAction")
			name := uniqueName("wf-dl-full-roundtrip")
			workflowID := uniqueName("wfid-dl-full")
			dl := models.DetectedLabels{PDBProtected: true, GitOpsTool: "flux"}
			rw := newWF(name, at, wfFixture{
				Severity:       []string{"critical", "high"},
				CustomLabels:   map[string]string{"team": "platform"},
				DetectedLabels: &dl,
			})
			createWF(rw)
			rw.Status.WorkflowID = workflowID
			Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

			retrieved := getByID(workflowID)
			Expect(retrieved.Labels.Severity).To(ConsistOf("critical", "high"))
			Expect(retrieved.CustomLabels).To(HaveKey("team"))
			Expect(retrieved.DetectedLabels.PDBProtected).To(BeTrue())
			Expect(retrieved.DetectedLabels.GitOpsTool).To(Equal("flux"))
			Expect(retrieved.Description.What).To(ContainSubstring(name))
		})
	})

	// ========================================
	// CNV-specific detected-label scoring/ranking (#1378)
	// ========================================
	Describe("CNV detected-label scoring and ranking (#1378)", func() {
		cnvFixture := func(f wfFixture) wfFixture {
			if len(f.Severity) == 0 {
				f.Severity = []string{"critical", "high"}
			}
			if len(f.Component) == 0 {
				f.Component = []string{"kubevirt.io/v1/VirtualMachine"}
			}
			if len(f.Environment) == 0 {
				f.Environment = []string{"production", "staging"}
			}
			return f
		}

		It("IT-KA-1677-1378-001: virtualMachine=true ranks CNV matches first [BR-WORKFLOW-004]", func() {
			at := uniquePascalName("CNVVMAction")
			cnvName := uniqueName("wf-cnv-full-match")
			genericName := uniqueName("wf-cnv-generic-kubevirt")

			fullCNV := models.DetectedLabels{VirtualMachine: true, LiveMigratable: true, CDIManaged: true, StorageBackend: "odf-ceph"}
			createWF(newWF(cnvName, at, cnvFixture(wfFixture{DetectedLabels: &fullCNV})))
			createWF(newWF(genericName, at, cnvFixture(wfFixture{DetectedLabels: &models.DetectedLabels{}})))

			filters := &models.WorkflowDiscoveryFilters{
				Severity: "critical", Component: "kubevirt.io/v1/VirtualMachine", Environment: "production", Priority: "P1",
				DetectedLabels: &models.DetectedLabels{VirtualMachine: true},
			}

			var results []models.RemediationWorkflow
			Eventually(func() (int, error) {
				var total int
				var err error
				results, total, err = catalog.ListWorkflowsByActionType(ctx, at, filters, 0, 100)
				return total, err
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(2), "both kubevirt workflows should match the VM discovery context")

			Expect(results[0].WorkflowName).To(Equal(cnvName), "CNV workflow with virtualMachine=true should rank first due to detected-label boost")
			Expect(results[0].DetectedLabels.VirtualMachine).To(BeTrue())
			Expect(results[1].DetectedLabels.IsEmpty()).To(BeTrue(), "generic kubevirt workflow should remain discoverable without CNV requirements")
		})

		It("IT-KA-1677-1378-002: storageBackend=odf-ceph ranks an exact match above a wildcard match [BR-WORKFLOW-004]", func() {
			at := uniquePascalName("CNVStorageAction")
			exactName := uniqueName("wf-cnv-exact-odf-ceph")
			wildcardName := uniqueName("wf-cnv-wildcard-storage")

			createWF(newWF(exactName, at, cnvFixture(wfFixture{DetectedLabels: &models.DetectedLabels{VirtualMachine: true, StorageBackend: "odf-ceph"}})))
			createWF(newWF(wildcardName, at, cnvFixture(wfFixture{DetectedLabels: &models.DetectedLabels{VirtualMachine: true, StorageBackend: "*"}})))

			filters := &models.WorkflowDiscoveryFilters{
				Severity: "critical", Component: "kubevirt.io/v1/VirtualMachine", Environment: "production", Priority: "P1",
				DetectedLabels: &models.DetectedLabels{StorageBackend: "odf-ceph"},
			}

			var results []models.RemediationWorkflow
			Eventually(func() (int, error) {
				var total int
				var err error
				results, total, err = catalog.ListWorkflowsByActionType(ctx, at, filters, 0, 100)
				return total, err
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(2), "exact and wildcard storageBackend workflows should both match odf-ceph")

			Expect(results[0].WorkflowName).To(Equal(exactName), "exact storageBackend=odf-ceph workflow should outrank the wildcard workflow")
			Expect(results[0].DetectedLabels.StorageBackend).To(Equal("odf-ceph"))
			Expect(results[1].DetectedLabels.StorageBackend).To(Equal("*"))
		})

		It("IT-KA-1677-1378-003: fixture parse -> serialize -> extract -> query preserves all 4 CNV fields [BR-WORKFLOW-004]", func() {
			at := uniquePascalName("CNVFixtureAction")
			schemaParser := schema.NewParser()
			rawFixture := testutil.LoadWorkflowFixture("cnv-vm-boot-failure")

			parsedSchema, err := schemaParser.ParseAndValidate(rawFixture)
			Expect(err).ToNot(HaveOccurred(), "CNV fixture should pass schema validation")

			extracted, err := schemaParser.ExtractDetectedLabels(parsedSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(extracted.VirtualMachine).To(BeTrue())
			Expect(extracted.LiveMigratable).To(BeTrue())
			Expect(extracted.CDIManaged).To(BeTrue())
			Expect(extracted.StorageBackend).To(Equal("odf-ceph"))

			serialized, err := extracted.SerializeLabels()
			Expect(err).ToNot(HaveOccurred())
			var roundtripped models.DetectedLabels
			Expect(json.Unmarshal(serialized, &roundtripped)).To(Succeed())
			Expect(roundtripped.VirtualMachine).To(Equal(extracted.VirtualMachine))
			Expect(roundtripped.LiveMigratable).To(Equal(extracted.LiveMigratable))
			Expect(roundtripped.CDIManaged).To(Equal(extracted.CDIManaged))
			Expect(roundtripped.StorageBackend).To(Equal(extracted.StorageBackend))

			fixtureName := uniqueName("wf-cnv-fixture")
			createWF(newWF(fixtureName, at, cnvFixture(wfFixture{
				DetectedLabels: &models.DetectedLabels{
					VirtualMachine: extracted.VirtualMachine,
					LiveMigratable: extracted.LiveMigratable,
					CDIManaged:     extracted.CDIManaged,
					StorageBackend: extracted.StorageBackend,
				},
			})))

			filters := &models.WorkflowDiscoveryFilters{
				Severity: "critical", Component: "kubevirt.io/v1/VirtualMachine", Environment: "production", Priority: "P1",
				DetectedLabels: &models.DetectedLabels{
					VirtualMachine: true, LiveMigratable: true, CDIManaged: true, StorageBackend: "odf-ceph",
				},
			}
			var results []models.RemediationWorkflow
			Eventually(func() (int, error) {
				var total int
				var err error
				results, total, err = catalog.ListWorkflowsByActionType(ctx, at, filters, 0, 100)
				return total, err
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(1))
			Expect(strings.HasPrefix(results[0].WorkflowName, fixtureName)).To(BeTrue())
			Expect(results[0].DetectedLabels.VirtualMachine).To(BeTrue())
			Expect(results[0].DetectedLabels.LiveMigratable).To(BeTrue())
			Expect(results[0].DetectedLabels.CDIManaged).To(BeTrue())
			Expect(results[0].DetectedLabels.StorageBackend).To(Equal("odf-ceph"))
		})
	})
})
