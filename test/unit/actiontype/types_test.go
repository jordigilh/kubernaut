/*
Copyright 2025 Jordi Gil.

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

package actiontype

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	"sigs.k8s.io/yaml"
)

func projectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..")
}

func validActionType() *atv1alpha1.ActionType {
	return &atv1alpha1.ActionType{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kubernaut.ai/v1alpha1",
			Kind:       "ActionType",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restart-pod",
			Namespace: "kubernaut-system",
		},
		Spec: atv1alpha1.ActionTypeSpec{
			Name: "RestartPod",
			Description: atv1alpha1.ActionTypeDescription{
				What:         "Deletes and restarts a failing pod to clear transient errors.",
				WhenToUse:    "When a pod is in CrashLoopBackOff or unresponsive state.",
				WhenNotToUse: "When the issue is caused by misconfiguration that persists across restarts.",
				Preconditions: "Verify the pod is not handling in-flight requests.",
			},
		},
	}
}

var _ = Describe("ActionType CRD Types — BR-WORKFLOW-007", func() {

	// UT-AT-001: Unmarshal valid ActionType YAML into Go struct
	Describe("UT-AT-001: YAML Unmarshalling", func() {
		It("should unmarshal a valid ActionType YAML into all spec and status fields", func() {
			yamlData := `
apiVersion: kubernaut.ai/v1alpha1
kind: ActionType
metadata:
  name: restart-pod
  namespace: kubernaut-system
spec:
  name: RestartPod
  description:
    what: "Deletes and restarts a failing pod"
    whenToUse: "Pod is in CrashLoopBackOff"
    whenNotToUse: "Misconfiguration that persists"
    preconditions: "No in-flight requests"
status:
  registered: true
  registeredBy: "system:serviceaccount:kubernaut-system:authwebhook"
  activeWorkflowCount: 3
  catalogStatus: "Active"
  previouslyExisted: false
`
			var at atv1alpha1.ActionType
			err := yaml.Unmarshal([]byte(yamlData), &at)
			Expect(err).NotTo(HaveOccurred())

			Expect(at.Spec.Name).To(Equal("RestartPod"))
			Expect(at.Spec.Description.What).To(Equal("Deletes and restarts a failing pod"))
			Expect(at.Spec.Description.WhenToUse).To(Equal("Pod is in CrashLoopBackOff"))
			Expect(at.Spec.Description.WhenNotToUse).To(Equal("Misconfiguration that persists"))
			Expect(at.Spec.Description.Preconditions).To(Equal("No in-flight requests"))

			Expect(at.Status.Registered).To(BeTrue())
			Expect(at.Status.RegisteredBy).To(Equal("system:serviceaccount:kubernaut-system:authwebhook"))
			Expect(at.Status.ActiveWorkflowCount).To(Equal(3))
			Expect(at.Status.CatalogStatus).To(Equal(sharedtypes.CatalogStatusActive))
			Expect(at.Status.PreviouslyExisted).To(BeFalse())
		})
	})

	// UT-AT-002: ActionType implements runtime.Object and can be added to a scheme
	Describe("UT-AT-002: Scheme Registration", func() {
		It("should register ActionType and ActionTypeList in a scheme", func() {
			s := k8sruntime.NewScheme()
			err := atv1alpha1.AddToScheme(s)
			Expect(err).NotTo(HaveOccurred())

			gvk := atv1alpha1.GroupVersion.WithKind("ActionType")
			obj, err := s.New(gvk)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).To(BeAssignableToTypeOf(&atv1alpha1.ActionType{}))

			listGVK := atv1alpha1.GroupVersion.WithKind("ActionTypeList")
			listObj, err := s.New(listGVK)
			Expect(err).NotTo(HaveOccurred())
			Expect(listObj).To(BeAssignableToTypeOf(&atv1alpha1.ActionTypeList{}))
		})

		It("should confirm GroupVersion is kubernaut.ai/v1alpha1", func() {
			Expect(atv1alpha1.GroupVersion.Group).To(Equal("kubernaut.ai"))
			Expect(atv1alpha1.GroupVersion.Version).To(Equal("v1alpha1"))
		})

		It("should be compatible with controller-runtime SchemeBuilder", func() {
			builder := &scheme.Builder{GroupVersion: atv1alpha1.GroupVersion}
			builder.Register(&atv1alpha1.ActionType{}, &atv1alpha1.ActionTypeList{})
			s := k8sruntime.NewScheme()
			err := builder.AddToScheme(s)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// UT-AT-003: DeepCopy round-trip
	Describe("UT-AT-003: DeepCopy", func() {
		It("should produce an identical but independent copy", func() {
			now := metav1.NewTime(time.Now().Truncate(time.Second))
			original := validActionType()
			original.Status = atv1alpha1.ActionTypeStatus{
				Registered:          true,
				RegisteredAt:        &now,
				RegisteredBy:        "system:serviceaccount:kubernaut-system:authwebhook",
				PreviouslyExisted:   true,
				ActiveWorkflowCount: 5,
				CatalogStatus:       sharedtypes.CatalogStatusActive,
			}

			copied := original.DeepCopy()

			Expect(copied.Spec).To(Equal(original.Spec))
			Expect(copied.Status).To(Equal(original.Status))
			Expect(copied.Name).To(Equal(original.Name))

			// Mutating the copy must not affect the original
			copied.Spec.Name = "Mutated"
			copied.Status.ActiveWorkflowCount = 99
			Expect(original.Spec.Name).To(Equal("RestartPod"))
			Expect(original.Status.ActiveWorkflowCount).To(Equal(5))
		})

		It("should deep copy ActionTypeList", func() {
			list := &atv1alpha1.ActionTypeList{
				Items: []atv1alpha1.ActionType{*validActionType()},
			}
			copied := list.DeepCopy()
			Expect(copied.Items).To(HaveLen(1))
			copied.Items[0].Spec.Name = "Mutated"
			Expect(list.Items[0].Spec.Name).To(Equal("RestartPod"))
		})
	})

	// UT-AT-004: JSON round-trip
	Describe("UT-AT-004: JSON Serialization", func() {
		It("should marshal and unmarshal without data loss", func() {
			original := validActionType()
			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var roundTripped atv1alpha1.ActionType
			err = json.Unmarshal(data, &roundTripped)
			Expect(err).NotTo(HaveOccurred())

			Expect(roundTripped.Spec.Name).To(Equal(original.Spec.Name))
			Expect(roundTripped.Spec.Description).To(Equal(original.Spec.Description))
		})

		It("should omit optional status fields when empty", func() {
			at := validActionType()
			data, err := json.Marshal(at)
			Expect(err).NotTo(HaveOccurred())

			jsonStr := string(data)
			Expect(jsonStr).NotTo(ContainSubstring(`"activeWorkflowCount"`))
			Expect(jsonStr).NotTo(ContainSubstring(`"catalogStatus"`))
			Expect(jsonStr).NotTo(ContainSubstring(`"registeredAt"`))
		})
	})

	// UT-AT-005: Generated CRD manifest contains expected printer columns
	Describe("UT-AT-005: CRD Manifest Printer Columns", func() {
		It("should define the expected printer columns", func() {
			crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_actiontypes.yaml")
			data, err := os.ReadFile(crdPath)
			Expect(err).NotTo(HaveOccurred(), "CRD manifest should exist at config/crd/bases/")

			crdYAML := string(data)

			Expect(crdYAML).To(ContainSubstring("name: Action Type"))
			Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.name"))

			Expect(crdYAML).To(ContainSubstring("name: Workflows"))
			Expect(crdYAML).To(ContainSubstring("jsonPath: .status.activeWorkflowCount"))
			Expect(crdYAML).To(ContainSubstring("type: integer"))

			Expect(crdYAML).To(ContainSubstring("name: Registered"))
			Expect(crdYAML).To(ContainSubstring("jsonPath: .status.registered"))
			Expect(crdYAML).To(ContainSubstring("type: boolean"))

			Expect(crdYAML).To(ContainSubstring("name: Age"))
			Expect(crdYAML).To(ContainSubstring("jsonPath: .metadata.creationTimestamp"))

			Expect(crdYAML).To(ContainSubstring("name: Description"))
			Expect(crdYAML).To(ContainSubstring("jsonPath: .spec.description.what"))
			Expect(crdYAML).To(ContainSubstring("priority: 1"))
		})
	})

	// UT-AT-006: Generated CRD manifest contains selectableFields
	Describe("UT-AT-006: CRD Manifest Selectable Fields", func() {
		It("should define .spec.name as a selectable field", func() {
			crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_actiontypes.yaml")
			data, err := os.ReadFile(crdPath)
			Expect(err).NotTo(HaveOccurred())

			crdYAML := string(data)
			Expect(crdYAML).To(ContainSubstring("selectableFields:"))

			selectableIdx := strings.Index(crdYAML, "selectableFields:")
			Expect(selectableIdx).To(BeNumerically(">", 0))
			selectableSection := crdYAML[selectableIdx : selectableIdx+100]
			Expect(selectableSection).To(ContainSubstring(".spec.name"))
		})
	})

	// UT-AT-007: CRD short name
	Describe("UT-AT-007: CRD Short Name", func() {
		It("should define 'at' as a short name", func() {
			crdPath := filepath.Join(projectRoot(), "config", "crd", "bases", "kubernaut.ai_actiontypes.yaml")
			data, err := os.ReadFile(crdPath)
			Expect(err).NotTo(HaveOccurred())

			crdYAML := string(data)
			Expect(crdYAML).To(ContainSubstring("shortNames:"))
			Expect(crdYAML).To(ContainSubstring("- at"))
		})
	})

	// UT-AT-008: Status defaults to zero values
	Describe("UT-AT-008: Status Zero Values", func() {
		It("should have zero-value status fields before webhook populates them", func() {
			at := validActionType()

			Expect(at.Status.Registered).To(BeFalse())
			Expect(at.Status.RegisteredAt).To(BeNil())
			Expect(at.Status.RegisteredBy).To(BeEmpty())
			Expect(at.Status.PreviouslyExisted).To(BeFalse())
			Expect(at.Status.ActiveWorkflowCount).To(Equal(0))
			Expect(at.Status.CatalogStatus).To(BeEmpty())
		})
	})
})
