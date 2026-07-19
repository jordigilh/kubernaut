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

package enrichment_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newCNVScheme() *runtime.Scheme {
	scheme := newFullScheme()
	_ = storagev1.AddToScheme(scheme)
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"},
		{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstance"},
		{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstanceMigration"},
		{Group: "cdi.kubevirt.io", Version: "v1beta1", Kind: "DataVolume"},
	} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		listGVK := gvk
		listGVK.Kind += "List"
		scheme.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
	}
	return scheme
}

func newCNVTestMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "autoscaling", Version: "v2"},
		{Group: "policy", Version: "v1"},
		{Group: "networking.k8s.io", Version: "v1"},
		{Group: "kubevirt.io", Version: "v1"},
		{Group: "cdi.kubevirt.io", Version: "v1beta1"},
		{Group: "storage.k8s.io", Version: "v1"},
	})
	// Standard kinds (same as newTestMapper)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolume"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ResourceQuota"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"}, meta.RESTScopeRoot)
	// CNV kinds
	m.Add(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstance"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstanceMigration"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "cdi.kubevirt.io", Version: "v1beta1", Kind: "DataVolume"}, meta.RESTScopeNamespace)
	return m
}

func makeUnstructuredVM(name, evictionStrategy string) *unstructured.Unstructured {
	vm := &unstructured.Unstructured{}
	vm.SetGroupVersionKind(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"})
	vm.SetName(name)
	vm.SetNamespace("cnv-ns")
	spec := map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{},
		},
	}
	if evictionStrategy != "" {
		spec["template"].(map[string]interface{})["spec"].(map[string]interface{})["evictionStrategy"] = evictionStrategy
	}
	vm.Object["spec"] = spec
	return vm
}

var _ = Describe("CNV DetectedLabels Detection — #1378", func() {

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ═══════════════════════════════════════════════════════════════
	// detectVirtualMachine — owner chain walk
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-1378-001: VirtualMachine in owner chain", func() {
		It("should detect virtualMachine=true", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("test-vm", "")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "test-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-test-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue(), "VirtualMachine in owner chain should trigger virtualMachine=true")
		})
	})

	Describe("UT-KA-1378-002: VirtualMachineInstance as target kind", func() {
		It("should detect virtualMachine=true", func() {
			scheme := newCNVScheme()
			vmi := &unstructured.Unstructured{}
			vmi.SetGroupVersionKind(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstance"})
			vmi.SetName("test-vmi")
			vmi.SetNamespace("cnv-ns")
			vmi.Object["spec"] = map[string]interface{}{}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vmi)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			labels, _, err := detector.DetectLabels(ctx, "VirtualMachineInstance", "test-vmi", "cnv-ns", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue(), "VirtualMachineInstance as target kind should trigger virtualMachine=true")
		})
	})

	Describe("UT-KA-1378-003: DataVolume in owner chain", func() {
		It("should detect virtualMachine=true", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("test-vm", "")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "DataVolume", Name: "test-dv", Namespace: "cnv-ns"},
				{Kind: "VirtualMachine", Name: "test-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "PersistentVolumeClaim", "test-dv", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue(), "DataVolume in owner chain should trigger virtualMachine=true")
		})
	})

	Describe("UT-KA-1378-004: Non-VM workload (Deployment)", func() {
		It("should not detect virtualMachine", func() {
			scheme := newCNVScheme()
			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("api-server")
			deploy.SetNamespace("production")
			deploy.Object["spec"] = map[string]interface{}{}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "api-server", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeFalse(), "Deployment should not trigger virtualMachine")
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// detectLiveMigratable — VM evictionStrategy
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-1378-005: VM with evictionStrategy=LiveMigrate", func() {
		It("should detect liveMigratable=true", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("migrate-vm", "LiveMigrate")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "migrate-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-migrate-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue())
			Expect(labels.LiveMigratable).To(BeTrue(), "evictionStrategy=LiveMigrate should trigger liveMigratable=true")
		})
	})

	Describe("UT-KA-1378-006: VM without evictionStrategy", func() {
		It("should not detect liveMigratable", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("no-migrate-vm", "")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "no-migrate-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-no-migrate-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue())
			Expect(labels.LiveMigratable).To(BeFalse(), "VM without evictionStrategy should not trigger liveMigratable")
		})
	})

	Describe("UT-KA-1378-007: Non-VM workload skips liveMigratable detection", func() {
		It("should not detect liveMigratable and no extra API call", func() {
			scheme := newCNVScheme()
			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("api-server")
			deploy.SetNamespace("production")
			deploy.Object["spec"] = map[string]interface{}{}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "api-server", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.LiveMigratable).To(BeFalse())
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// detectCDIManaged — PVC annotations
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-1378-008: PVC with CDI import annotation", func() {
		It("should detect cdiManaged=true", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("cdi-vm", "")
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dv",
					Namespace: "cnv-ns",
					Annotations: map[string]string{
						"cdi.kubevirt.io/storage.import.endpoint": "https://example.com/disk.qcow2",
					},
				},
			}
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm, pvc)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "cdi-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-cdi-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.CDIManaged).To(BeTrue(), "PVC with cdi.kubevirt.io annotation should trigger cdiManaged=true")
		})
	})

	Describe("UT-KA-1378-009: PVC without CDI annotations", func() {
		It("should not detect cdiManaged", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("plain-vm", "")
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-disk",
					Namespace: "cnv-ns",
				},
			}
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm, pvc)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "plain-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-plain-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.CDIManaged).To(BeFalse(), "PVC without CDI annotations should not trigger cdiManaged")
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// detectStorageBackend — PVC -> StorageClass -> provisioner
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-1378-010: PVC with rbd.csi.ceph.com provisioner", func() {
		It("should detect storageBackend=odf-ceph", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("odf-vm", "")
			scName := "ocs-storagecluster-ceph-rbd"
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-disk",
					Namespace: "cnv-ns",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
				},
			}
			sc := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: scName},
				Provisioner: "openshift-storage.rbd.csi.ceph.com",
			}
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm, pvc, sc)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "odf-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-odf-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.StorageBackend).To(Equal("odf-ceph"), "rbd.csi.ceph.com provisioner should map to odf-ceph")
		})
	})

	Describe("UT-KA-1378-011: PVC with topolvm.io provisioner", func() {
		It("should detect storageBackend=lvms", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("lvms-vm", "")
			scName := "lvms-vg1"
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-disk",
					Namespace: "cnv-ns",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
				},
			}
			sc := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: scName},
				Provisioner: "topolvm.io",
			}
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm, pvc, sc)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "lvms-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-lvms-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.StorageBackend).To(Equal("lvms"), "topolvm.io provisioner should map to lvms")
		})
	})

	Describe("UT-KA-1378-012: PVC with kubernetes.io/no-provisioner", func() {
		It("should detect storageBackend=local", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("local-vm", "")
			scName := "local-storage"
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-disk",
					Namespace: "cnv-ns",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
				},
			}
			sc := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: scName},
				Provisioner: "kubernetes.io/no-provisioner",
			}
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm, pvc, sc)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "local-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-local-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.StorageBackend).To(Equal("local"), "kubernetes.io/no-provisioner should map to local")
		})
	})

	Describe("UT-KA-1378-013: PVC with unknown provisioner", func() {
		It("should leave storageBackend empty", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("unknown-vm", "")
			scName := "custom-storage"
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-disk",
					Namespace: "cnv-ns",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
				},
			}
			sc := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: scName},
				Provisioner: "vendor.example.com/custom-csi",
			}
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm, pvc, sc)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "unknown-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-unknown-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.StorageBackend).To(BeEmpty(), "Unknown provisioner should leave storageBackend empty")
		})
	})

	Describe("UT-KA-1378-014: No PVCs in namespace", func() {
		It("should leave storageBackend empty and cdiManaged false", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("nopvc-vm", "")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "nopvc-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-nopvc-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.StorageBackend).To(BeEmpty())
			Expect(labels.CDIManaged).To(BeFalse())
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// RESTMapper pre-check — non-CNV cluster
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-1378-015: CNV CRDs not installed (RESTMapper pre-check)", func() {
		It("should leave all 4 CNV fields as zero values and NOT in FailedDetections", func() {
			scheme := newFullScheme()
			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("api-server")
			deploy.SetNamespace("production")
			deploy.Object["spec"] = map[string]interface{}{}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "api-server", "production", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeFalse())
			Expect(labels.LiveMigratable).To(BeFalse())
			Expect(labels.CDIManaged).To(BeFalse())
			Expect(labels.StorageBackend).To(BeEmpty())
			for _, fd := range labels.FailedDetections {
				Expect(fd).NotTo(BeElementOf("virtualMachine", "liveMigratable", "cdiManaged", "storageBackend"),
					"CNV fields should NOT be in FailedDetections on non-CNV cluster")
			}
		})
	})

	// ═══════════════════════════════════════════════════════════════
	// FailedDetections tracking for API errors
	// ═══════════════════════════════════════════════════════════════

	Describe("UT-KA-1378-016: VM GET fails (simulated RBAC denied)", func() {
		It("should add liveMigratable to FailedDetections", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("rbac-vm", "LiveMigrate")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)

			vmGetCount := 0
			dynClient.PrependReactor("get", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				vmGetCount++
				if vmGetCount >= 2 {
					return true, nil, fmt.Errorf("simulated RBAC denied: forbidden")
				}
				return false, nil, nil
			})

			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "rbac-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-rbac-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue(), "owner chain walk is local, should still detect VM")
			Expect(labels.FailedDetections).To(ContainElement("liveMigratable"),
				"failed VM GET should add liveMigratable to FailedDetections")
		})
	})

	Describe("UT-KA-1378-017: PVC LIST fails (simulated timeout)", func() {
		It("should add cdiManaged and storageBackend to FailedDetections", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("fail-pvc-vm", "")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)
			dynClient.PrependReactor("list", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("simulated timeout: context deadline exceeded")
			})
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "fail-pvc-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-fail-pvc-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.CDIManaged).To(BeFalse())
			Expect(labels.StorageBackend).To(BeEmpty())
			Expect(labels.FailedDetections).To(ContainElement("cdiManaged"),
				"PVC LIST failure should add cdiManaged to FailedDetections")
			Expect(labels.FailedDetections).To(ContainElement("storageBackend"),
				"PVC LIST failure should add storageBackend to FailedDetections")
		})
	})

	Describe("UT-KA-1378-018: VirtualMachineInstanceMigration as target kind", func() {
		It("should detect virtualMachine=true", func() {
			scheme := newCNVScheme()
			vmim := &unstructured.Unstructured{}
			vmim.SetGroupVersionKind(schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachineInstanceMigration"})
			vmim.SetName("test-vmim")
			vmim.SetNamespace("cnv-ns")
			vmim.Object["spec"] = map[string]interface{}{}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vmim)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			labels, _, err := detector.DetectLabels(ctx, "VirtualMachineInstanceMigration", "test-vmim", "cnv-ns", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue(), "VirtualMachineInstanceMigration should trigger virtualMachine=true")
		})
	})
})
