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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

var _ = Describe("CNV DetectedLabels Integration — #1378", Label("it", "ka", "cnv", "1378"), func() {

	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("IT-KA-1378-001: Full DetectLabels with CNV cluster fixtures", func() {
		It("should detect all 4 CNV labels in a single DetectLabels call [BR-WORKFLOW-018]", func() {
			scheme := newCNVScheme()

			vm := makeUnstructuredVM("full-cnv-vm", "LiveMigrate")

			dv := &unstructured.Unstructured{}
			dv.SetGroupVersionKind(schema.GroupVersionKind{Group: "cdi.kubevirt.io", Version: "v1beta1", Kind: "DataVolume"})
			dv.SetName("test-dv")
			dv.SetNamespace("cnv-ns")
			dv.Object["spec"] = map[string]interface{}{
				"source": map[string]interface{}{
					"blank": map[string]interface{}{},
				},
			}

			scName := "ocs-storagecluster-ceph-rbd"
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dv",
					Namespace: "cnv-ns",
					Annotations: map[string]string{
						"cdi.kubevirt.io/storage.import.endpoint": "https://example.com/disk.qcow2",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
				},
			}
			sc := &storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: scName},
				Provisioner: "openshift-storage.rbd.csi.ceph.com",
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm, dv, pvc, sc)
			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "full-cnv-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-full-cnv-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue(), "VirtualMachine in owner chain should trigger virtualMachine=true")
			Expect(labels.LiveMigratable).To(BeTrue(), "evictionStrategy=LiveMigrate should trigger liveMigratable=true")
			Expect(labels.CDIManaged).To(BeTrue(), "PVC with CDI annotation should trigger cdiManaged=true")
			Expect(labels.StorageBackend).To(Equal("odf-ceph"), "rbd.csi.ceph.com provisioner should map to odf-ceph")
		})
	})

	Describe("IT-KA-1378-002: DetectLabels on non-CNV cluster", func() {
		It("should leave all 4 CNV fields zero and omit them from FailedDetections [BR-WORKFLOW-018]", func() {
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

	Describe("IT-KA-1378-003: DetectLabels with RBAC-denied PVC LIST", func() {
		It("should detect virtualMachine=true and record cdiManaged+storageBackend failures [BR-WORKFLOW-018]", func() {
			scheme := newCNVScheme()
			vm := makeUnstructuredVM("rbac-pvc-vm", "LiveMigrate")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, vm)
			dynClient.PrependReactor("list", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("simulated RBAC denied: forbidden")
			})

			detector := enrichment.NewLabelDetector(dynClient, newCNVTestMapper(), logr.Discard())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "VirtualMachine", Name: "rbac-pvc-vm", Namespace: "cnv-ns"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Pod", "virt-launcher-rbac-pvc-vm-abc", "cnv-ns", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels.VirtualMachine).To(BeTrue(), "owner chain walk is local, should still detect VM")
			Expect(labels.CDIManaged).To(BeFalse())
			Expect(labels.StorageBackend).To(BeEmpty())
			Expect(labels.FailedDetections).To(ContainElement("cdiManaged"),
				"PVC LIST failure should add cdiManaged to FailedDetections")
			Expect(labels.FailedDetections).To(ContainElement("storageBackend"),
				"PVC LIST failure should add storageBackend to FailedDetections")
		})
	})
})
