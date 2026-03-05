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

package gateway

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func boolPtr(b bool) *bool { return &b }

var _ = Describe("K8sOwnerResolver - Owner chain resolution with real K8s objects (#270)", func() {
	const namespace = "demo-memory-leak"

	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		fakeClient client.Client
		resolver   *adapters.K8sOwnerResolver
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
	})

	setup := func(objs ...client.Object) {
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			Build()
		resolver = adapters.NewK8sOwnerResolver(fakeClient)
	}

	Describe("Pod -> ReplicaSet -> Deployment (BR-GATEWAY-004)", func() {
		It("should resolve to the Deployment", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app", Namespace: namespace,
					UID: k8stypes.UID("deploy-uid"),
				},
			}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-587f69c664", Namespace: namespace,
					UID: k8stypes.UID("rs-uid"),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "leaky-app",
						UID: k8stypes.UID("deploy-uid"), Controller: boolPtr(true),
					}},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-587f69c664-mp86z", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-587f69c664",
						UID: k8stypes.UID("rs-uid"), Controller: boolPtr(true),
					}},
				},
			}

			setup(deploy, rs, pod)

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", pod.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("Deployment"))
			Expect(ownerName).To(Equal("leaky-app"))
		})
	})

	Describe("Pod -> StatefulSet", func() {
		It("should resolve to the StatefulSet", func() {
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "redis", Namespace: namespace,
					UID: k8stypes.UID("sts-uid"),
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "redis-0", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "StatefulSet", Name: "redis",
						UID: k8stypes.UID("sts-uid"), Controller: boolPtr(true),
					}},
				},
			}

			setup(sts, pod)

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "redis-0")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("StatefulSet"))
			Expect(ownerName).To(Equal("redis"))
		})
	})

	Describe("Pod -> DaemonSet", func() {
		It("should resolve to the DaemonSet", func() {
			ds := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fluentd", Namespace: namespace,
					UID: k8stypes.UID("ds-uid"),
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fluentd-abc12", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "DaemonSet", Name: "fluentd",
						UID: k8stypes.UID("ds-uid"), Controller: boolPtr(true),
					}},
				},
			}

			setup(ds, pod)

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "fluentd-abc12")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("DaemonSet"))
			Expect(ownerName).To(Equal("fluentd"))
		})
	})

	Describe("Pod -> Job -> CronJob", func() {
		It("should resolve to the CronJob", func() {
			cj := &batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "backup", Namespace: namespace,
					UID: k8stypes.UID("cj-uid"),
				},
			}
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "backup-28450000", Namespace: namespace,
					UID: k8stypes.UID("job-uid"),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1", Kind: "CronJob", Name: "backup",
						UID: k8stypes.UID("cj-uid"), Controller: boolPtr(true),
					}},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "backup-28450000-abc12", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1", Kind: "Job", Name: "backup-28450000",
						UID: k8stypes.UID("job-uid"), Controller: boolPtr(true),
					}},
				},
			}

			setup(cj, job, pod)

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "backup-28450000-abc12")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("CronJob"))
			Expect(ownerName).To(Equal("backup"))
		})
	})

	Describe("Pod -> Job (standalone, no CronJob)", func() {
		It("should resolve to the Job", func() {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "migration", Namespace: namespace,
					UID: k8stypes.UID("job-uid"),
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "migration-xyz12", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1", Kind: "Job", Name: "migration",
						UID: k8stypes.UID("job-uid"), Controller: boolPtr(true),
					}},
				},
			}

			setup(job, pod)

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "migration-xyz12")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("Job"))
			Expect(ownerName).To(Equal("migration"))
		})
	})

	Describe("Standalone Pod (no ownerReferences)", func() {
		It("should return the Pod itself", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "debug-pod", Namespace: namespace,
				},
			}

			setup(pod)

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "debug-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("Pod"))
			Expect(ownerName).To(Equal("debug-pod"))
		})
	})

	Describe("Deleted Pod (not found)", func() {
		It("should return error when pod does not exist", func() {
			setup()

			_, _, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "ghost-pod")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to resolve owner"))
		})
	})

	Describe("Two pods from same Deployment produce same fingerprint (#270 bug scenario)", func() {
		It("should produce identical deployment-level fingerprints", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app", Namespace: namespace,
					UID: k8stypes.UID("deploy-uid"),
				},
			}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986", Namespace: namespace,
					UID: k8stypes.UID("rs-uid"),
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "leaky-app",
						UID: k8stypes.UID("deploy-uid"), Controller: boolPtr(true),
					}},
				},
			}
			podA := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986-whccw", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-544b75986",
						UID: k8stypes.UID("rs-uid"), Controller: boolPtr(true),
					}},
				},
			}
			podB := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986-jxtck", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-544b75986",
						UID: k8stypes.UID("rs-uid"), Controller: boolPtr(true),
					}},
				},
			}

			setup(deploy, rs, podA, podB)

			resourceA := types.ResourceIdentifier{Namespace: namespace, Kind: "Pod", Name: podA.Name}
			resourceB := types.ResourceIdentifier{Namespace: namespace, Kind: "Pod", Name: podB.Name}

			fpA := types.ResolveFingerprint(ctx, resolver, resourceA)
			fpB := types.ResolveFingerprint(ctx, resolver, resourceB)

			Expect(fpA).To(Equal(fpB),
				"Two pods from the same Deployment must produce identical fingerprints")

			expectedFP := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: namespace, Kind: "Deployment", Name: "leaky-app",
			})
			Expect(fpA).To(Equal(expectedFP),
				"Fingerprint should be SHA256(namespace:Deployment:leaky-app)")
		})
	})
})
