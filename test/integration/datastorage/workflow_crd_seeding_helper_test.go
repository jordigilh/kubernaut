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

package datastorage

import (
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // Ginkgo/Gomega convention
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // Ginkgo/Gomega convention

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// ========================================
// CRD-NATIVE SEEDING HELPER (Issue #1661 Phase F)
// ========================================
// DD-WORKFLOW-018: etcd is the sole source of truth for RemediationWorkflow
// definitions. workflowRepo.Create (Postgres INSERT) has zero production
// callers post-Phase-B -- AuthWebhook is the sole write path. Specs that used
// Create purely as a seeding mechanism (not as the subject under test) migrate
// to seedWorkflowCRD below, which creates a real RemediationWorkflow CRD via
// k8sClient and status-patches it the same way AuthWebhook would
// (infrastructure.SeedWorkflowContentViaDirectCRDCreation), so the shared
// sharedWorkflowCache observes it exactly as it would a real admission.
//
// Callers must wire the cache onto the workflow.Repository under test via
// newCachedWorkflowRepo (or workflowRepo.SetCache(sharedWorkflowCache)
// directly) -- otherwise List/GetByID/ListActions/ListWorkflowsByActionType
// fall back to Postgres, which a CRD-native-seeded workflow never touches.
// ========================================

// newCachedWorkflowRepo returns a *workflow.Repository wired to the shared
// per-process informer-backed cache (sharedWorkflowCache), so its
// List/GetByID/ListActions/ListWorkflowsByActionType/GetWorkflowWithContextFilters
// calls read seeded-via-CRD workflows instead of falling back to Postgres
// (which CRD-native seeding never writes to).
func newCachedWorkflowRepo() *workflow.Repository {
	r := workflow.NewRepository(db, logger)
	r.SetCache(sharedWorkflowCache)
	return r
}

// workflowCRDSpec collects the fields seedWorkflowCRD needs to build a
// RemediationWorkflow CRD. Mirrors the subset of models.WorkflowSchema that
// the migrated specs in this package actually vary; callers set only the
// fields relevant to their scenario -- zero-value fields keep
// testutil.NewTestWorkflowCRD's baseline defaults (severity=[critical],
// environment=[production], component=[v1/Pod], priority=P1).
type workflowCRDSpec struct {
	Name        string
	ActionType  string
	Engine      string // defaults to "tekton" via testutil.NewTestWorkflowCRD when empty
	Severity    []string
	Component   []string
	Environment []string
	Priority    string
	Cluster     []string
	// DetectedLabels uses models.DetectedLabels' bool/string fields (the
	// storage-layer shape, models.DetectedLabels crdDetectedLabelsToModel
	// unmarshals into) rather than the YAML-authoring DetectedLabelsSchema
	// (string-typed, tracks which fields were explicitly declared via
	// PopulatedFields) -- detectedLabelsSchema below converts, treating any
	// non-zero field as "populated" (sufficient here: an omitted JSON key and
	// an explicit zero value unmarshal identically back into
	// models.DetectedLabels).
	DetectedLabels *models.DetectedLabels
	CustomLabels   map[string]string
	EngineConfig   interface{}
	Parameters     []models.WorkflowParameter
}

// detectedLabelsSchema converts a models.DetectedLabels (bool/string,
// storage-layer shape) into a *models.DetectedLabelsSchema (string,
// YAML-authoring shape) suitable for models.WorkflowSchema.DetectedLabels,
// marking every non-zero field as populated so MarshalYAML emits it.
func detectedLabelsSchema(dl models.DetectedLabels) *models.DetectedLabelsSchema {
	s := &models.DetectedLabelsSchema{}
	populate := func(field, val string) {
		s.PopulatedFields = append(s.PopulatedFields, field)
		switch field {
		case "gitOpsManaged":
			s.GitOpsManaged = val
		case "gitOpsTool":
			s.GitOpsTool = val
		case "pdbProtected":
			s.PDBProtected = val
		case "hpaEnabled":
			s.HPAEnabled = val
		case "stateful":
			s.Stateful = val
		case "helmManaged":
			s.HelmManaged = val
		case "networkIsolated":
			s.NetworkIsolated = val
		case "serviceMesh":
			s.ServiceMesh = val
		case "virtualMachine":
			s.VirtualMachine = val
		case "liveMigratable":
			s.LiveMigratable = val
		case "cdiManaged":
			s.CDIManaged = val
		case "storageBackend":
			s.StorageBackend = val
		}
	}
	if dl.GitOpsManaged {
		populate("gitOpsManaged", "true")
	}
	if dl.GitOpsTool != "" {
		populate("gitOpsTool", dl.GitOpsTool)
	}
	if dl.PDBProtected {
		populate("pdbProtected", "true")
	}
	if dl.HPAEnabled {
		populate("hpaEnabled", "true")
	}
	if dl.Stateful {
		populate("stateful", "true")
	}
	if dl.HelmManaged {
		populate("helmManaged", "true")
	}
	if dl.NetworkIsolated {
		populate("networkIsolated", "true")
	}
	if dl.ServiceMesh != "" {
		populate("serviceMesh", dl.ServiceMesh)
	}
	if dl.VirtualMachine {
		populate("virtualMachine", "true")
	}
	if dl.LiveMigratable {
		populate("liveMigratable", "true")
	}
	if dl.CDIManaged {
		populate("cdiManaged", "true")
	}
	if dl.StorageBackend != "" {
		populate("storageBackend", dl.StorageBackend)
	}
	return s
}

// seedWorkflowCRD creates a RemediationWorkflow CRD from spec via k8sClient
// and status-patches it the same way AuthWebhook would (deterministic
// workflow_id/content_hash from pkg/shared/contenthash), so the shared
// informer cache observes it as a fully-admitted workflow. Returns the
// deterministic workflow_id (UUID string), for use with
// newCachedWorkflowRepo().GetByID or GetWorkflowWithContextFilters.
//
// Registers a DeferCleanup that deletes the CRD and blocks until the shared
// informer cache (sharedWorkflowCache) has observed the deletion. This is the
// CRD-native equivalent of the retired Postgres tests' testID-scoped
// `DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE ...`
// AfterEach cleanup (#1661 Phase F): sharedWorkflowCache and its backing
// namespace (workflowCRDNamespace) are shared across every spec in this
// suite for the life of the process, so without this cleanup, workflows
// seeded by one spec would remain visible to (and inflate exact-count
// assertions in) every spec that runs afterward.
func seedWorkflowCRD(spec workflowCRDSpec) string {
	engine := spec.Engine
	if engine == "" {
		engine = "tekton"
	}
	crd := testutil.NewTestWorkflowCRD(spec.Name, spec.ActionType, engine)

	if len(spec.Severity) > 0 {
		crd.Spec.Labels.Severity = spec.Severity
	}
	if len(spec.Component) > 0 {
		crd.Spec.Labels.Component = spec.Component
	}
	if len(spec.Environment) > 0 {
		crd.Spec.Labels.Environment = spec.Environment
	}
	if spec.Priority != "" {
		crd.Spec.Labels.Priority = spec.Priority
	}
	if len(spec.Cluster) > 0 {
		crd.Spec.Labels.Cluster = spec.Cluster
	}
	if spec.DetectedLabels != nil {
		crd.Spec.DetectedLabels = detectedLabelsSchema(*spec.DetectedLabels)
	}
	if len(spec.CustomLabels) > 0 {
		crd.Spec.CustomLabels = spec.CustomLabels
	}
	if spec.EngineConfig != nil {
		crd.Spec.Execution.EngineConfig = spec.EngineConfig
	}
	if len(spec.Parameters) > 0 {
		crd.Spec.Parameters = spec.Parameters
	}

	content := testutil.MarshalWorkflowCRD(crd)
	workflowID, err := infrastructure.SeedWorkflowContentViaDirectCRDCreation(ctx, k8sClient, workflowCRDNamespace, content, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "seedWorkflowCRD(%s) should create and status-patch the CRD", spec.Name)

	// The informer cache observes the create/status-patch asynchronously via
	// its watch stream; block here until it has, so callers can query
	// immediately after seedWorkflowCRD returns without a race.
	Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
		return sharedWorkflowCache.GetWorkflowByID(ctx, workflowID)
	}).WithTimeout(5*time.Second).WithPolling(100*time.Millisecond).ShouldNot(BeNil(),
		"seedWorkflowCRD: shared cache must observe %s before returning", spec.Name)

	DeferCleanup(func() {
		obj := &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: workflowCRDNamespace},
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).To(Succeed(),
			"seedWorkflowCRD cleanup: delete %s", spec.Name)
		Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
			return sharedWorkflowCache.GetWorkflowByID(ctx, workflowID)
		}).WithTimeout(5*time.Second).WithPolling(100*time.Millisecond).Should(BeNil(),
			"seedWorkflowCRD cleanup: shared cache must observe %s deletion before the next spec runs", spec.Name)
	})

	return workflowID
}

// deleteWorkflowAndWaitForSharedCache deletes a RemediationWorkflow CRD
// created directly via k8sClient (rather than through seedWorkflowCRD above)
// and blocks until sharedWorkflowCache -- the suite-wide cache every other
// spec's workflowRepo/wfCache reads through -- has observed the deletion.
//
// A bare `DeferCleanup(func(){ _ = k8sClient.Delete(ctx, rw) })` returns as
// soon as the apiserver acks the delete, well before the informer watch
// event reaches sharedWorkflowCache. Ginkgo then considers the spec
// "finished" and may immediately start the next one -- including a Serial
// discovery spec in another file (e.g.
// workflow_discovery_repository_test.go's IT-DS-017-001-001) querying with
// an empty/wildcard filter that matches *any* Active workflow regardless of
// action type or labels. That spec can transiently still observe this "already
// deleted" workflow in the cache and over-count it. Root-caused for the
// #1661 intermittent off-by-one/two failures in IT-DS-017-001-*, IT-DS-464-*,
// IT-DS-522-*, IT-DS-595-*, IT-DS-1511-* -- traced to workflow_cache_test.go,
// workflow_cache_repository_test.go, server_workflow_cache_wiring_test.go and
// actiontype_workflow_count_test.go's raw (non-cache-waiting) cleanups of
// RemediationWorkflow CRDs sharing this package's envtest apiserver.
func deleteWorkflowAndWaitForSharedCache(rw *rwv1alpha1.RemediationWorkflow) {
	Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, rw))).To(Succeed(),
		"deleteWorkflowAndWaitForSharedCache: delete %s", rw.Name)
	Eventually(func() (*rwv1alpha1.RemediationWorkflow, error) {
		return sharedWorkflowCache.GetWorkflow(ctx, rw.Name)
	}, 5*time.Second, 100*time.Millisecond).Should(BeNil(),
		"sharedWorkflowCache must observe %s's deletion before the next spec runs", rw.Name)
}
