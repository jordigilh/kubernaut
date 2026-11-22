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

package adapters_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
)

var _ = Describe("BR-GATEWAY-004 BR-SCOPE-002 authoritative owner GVK", func() {
	DescribeTable("UT-GW-2593-GVK-001 carries each identity's own version",
		func(inputVersion, inputKind, ownerVersion, ownerKind, ownerName string) {
			child := ownerGVKObject(inputVersion, inputKind, "child", ownerVersion, ownerKind, ownerName)
			objects := []client.Object{child}
			if ownerName != "" {
				objects = append(objects, ownerGVKObject(ownerVersion, ownerKind, ownerName, "", "", ""))
			}
			reader := fake.NewClientBuilder().WithObjects(objects...).Build()
			resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard())
			input := types.ResourceIdentifier{Namespace: "prod", Kind: inputKind, Name: "child", APIVersion: inputVersion}
			fp, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "remote", resolver, input, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			expected := input
			if ownerName != "" {
				expected.Kind, expected.Name, expected.APIVersion = ownerKind, ownerName, ownerVersion
			}
			Expect(resolved).To(Equal(expected))
			Expect(fp).To(Equal(types.CalculateClusterAwareFingerprint("remote", expected)))
		},
		Entry("changed-kind Pod to Deployment", "v1", "Pod", "apps/v1", "Deployment", "parent"),
		Entry("same-kind owner has a different group/version", "apps/v1", "Deployment", "example.io/v2", "Deployment", "parent"),
		Entry("standalone Deployment", "apps/v1", "Deployment", "", "", ""),
		Entry("standalone Pod", "v1", "Pod", "", "", ""),
		Entry("explicit unknown-to-static-table CRD", "example.io/v2", "Widget", "", "", ""),
	)

	DescribeTable("UT-GW-2593-GVK-002 rejects unknown or malformed owner authority",
		func(ownerVersion string) {
			child := ownerGVKObject("v1", "Pod", "child", ownerVersion, "Deployment", "parent")
			reader := fake.NewClientBuilder().WithObjects(child).Build()
			resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard())
			fp, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "remote", resolver,
				types.ResourceIdentifier{Namespace: "prod", Kind: "Pod", Name: "child", APIVersion: "v1"}, logr.Discard())
			Expect(err).To(HaveOccurred())
			Expect(fp).To(BeEmpty())
			Expect(resolved).To(BeZero())
		}, Entry("missing", ""), Entry("malformed", "bad/version/extra"))

	It("UT-GW-2593-GVK-003 preserves explicit version over registry preference", func() {
		registry, err := adapters.NewAPIResourceRegistry(newFakeDiscovery(standardResources()))
		Expect(err).NotTo(HaveOccurred())
		child := ownerGVKObject("apps/v1beta2", "Deployment", "child", "", "", "")
		reader := fake.NewClientBuilder().WithObjects(child).Build()
		resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard(), ownerchain.WithRegistry(registry))
		_, resolved, err := types.ResolveFingerprint(context.Background(), resolver,
			types.ResourceIdentifier{Namespace: "prod", Kind: "Deployment", Name: "child", APIVersion: "apps/v1beta2"}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.APIVersion).To(Equal("apps/v1beta2"))
	})

	It("UT-GW-2593-GVK-004 carries trust-but-verify reference authority", func() {
		cached := ownerGVKObject("v1", "Pod", "child", "", "", "")
		fresh := ownerGVKObject("v1", "Pod", "child", "apps/v1", "Deployment", "parent")
		cache := fake.NewClientBuilder().WithObjects(cached).Build()
		direct := fake.NewClientBuilder().WithObjects(fresh).Build()
		resolver := ownerchain.NewK8sOwnerResolver(cache, logr.Discard(), ownerchain.WithFallbackReader(direct))
		_, resolved, err := types.ResolveFingerprint(context.Background(), resolver,
			types.ResourceIdentifier{Namespace: "prod", Kind: "Pod", Name: "child", APIVersion: "v1"}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.APIVersion).To(Equal("apps/v1"))
		Expect(resolved.Name).To(Equal("parent"))
	})

	It("UT-GW-2593-GVK-005 rejects absent initial authority without discovery", func() {
		resolver := ownerchain.NewK8sOwnerResolver(fake.NewClientBuilder().Build(), logr.Discard())
		_, _, err := types.ResolveFingerprintWithCluster(context.Background(), "remote", resolver,
			types.ResourceIdentifier{Namespace: "prod", Kind: "Widget", Name: "child"}, logr.Discard())
		Expect(err).To(HaveOccurred())
	})

	It("UT-GW-2593-GVK-006 obtains absent initial version from discovery", func() {
		registry, err := adapters.NewAPIResourceRegistry(newFakeDiscovery(standardResources()))
		Expect(err).NotTo(HaveOccurred())
		reader := fake.NewClientBuilder().WithObjects(ownerGVKObject("v1", "Pod", "child", "", "", "")).Build()
		resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard(), ownerchain.WithRegistry(registry))
		_, resolved, err := types.ResolveFingerprint(context.Background(), resolver,
			types.ResourceIdentifier{Namespace: "prod", Kind: "Pod", Name: "child"}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.APIVersion).To(Equal("v1"))
	})

	It("UT-GW-2593-GVK-007 carries cache-miss fallback authority", func() {
		fresh := ownerGVKObject("v1", "Pod", "child", "batch/v1", "Job", "parent")
		resolver := ownerchain.NewK8sOwnerResolver(fake.NewClientBuilder().Build(), logr.Discard(),
			ownerchain.WithFallbackReader(fake.NewClientBuilder().WithObjects(fresh).Build()))
		_, resolved, err := types.ResolveFingerprint(context.Background(), resolver,
			types.ResourceIdentifier{Namespace: "prod", Kind: "Pod", Name: "child", APIVersion: "v1"}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Kind).To(Equal("Job"))
		Expect(resolved.APIVersion).To(Equal("batch/v1"))
	})

	It("UT-GW-2593-GVK-008 carries Service-selected Pod and controller authority", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "service", Namespace: "prod"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "test"}}}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "prod", Labels: map[string]string{"app": "test"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "parent", Controller: boolPtr(true)}}}}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, pod).Build()
		resolver := ownerchain.NewK8sOwnerResolver(reader, logr.Discard(), ownerchain.WithFallbackReader(reader))
		_, resolved, err := types.ResolveFingerprint(context.Background(), resolver,
			types.ResourceIdentifier{Namespace: "prod", Kind: "Service", Name: "service", APIVersion: "v1"}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Kind).To(Equal("Deployment"))
		Expect(resolved.Name).To(Equal("parent"))
		Expect(resolved.APIVersion).To(Equal("apps/v1"))
	})

	DescribeTable("UT-GW-2593-GVK-009 validates the maximum-depth owner's authority", func(finalVersion string, wantError bool) {
		objects := make([]client.Object, 0, ownerchain.MaxOwnerChainDepth)
		for i := 0; i < ownerchain.MaxOwnerChainDepth; i++ {
			version := "apps/v1"
			if i == ownerchain.MaxOwnerChainDepth-1 {
				version = finalVersion
			}
			objects = append(objects, ownerGVKObject("apps/v1", "Deployment", fmt.Sprint(i), version, "Deployment", fmt.Sprint(i+1)))
		}
		resolver := ownerchain.NewK8sOwnerResolver(fake.NewClientBuilder().WithObjects(objects...).Build(), logr.Discard())
		_, resolved, err := types.ResolveFingerprint(context.Background(), resolver,
			types.ResourceIdentifier{Namespace: "prod", Kind: "Deployment", Name: "0", APIVersion: "apps/v1"}, logr.Discard())
		if wantError {
			Expect(err).To(HaveOccurred())
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved.Name).To(Equal(fmt.Sprint(ownerchain.MaxOwnerChainDepth)))
		Expect(resolved.APIVersion).To(Equal(finalVersion))
	}, Entry("valid last reference", "apps/v1beta2", false), Entry("missing last reference version", "", true))

	It("UT-GW-2593-GVK-010 nil resolver preserves the complete source identity", func() {
		input := types.ResourceIdentifier{Namespace: "prod", Kind: "Widget", Name: "child", APIVersion: "example.io/v2"}
		_, resolved, err := types.ResolveFingerprint(context.Background(), nil, input, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(resolved).To(Equal(input))
	})

})

func ownerGVKObject(version, kind, name, ownerVersion, ownerKind, ownerName string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.FromAPIVersionAndKind(version, kind))
	obj.SetNamespace("prod")
	obj.SetName(name)
	if ownerName != "" {
		obj.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: ownerVersion, Kind: ownerKind, Name: ownerName, Controller: boolPtr(true)}})
	}
	return obj
}
