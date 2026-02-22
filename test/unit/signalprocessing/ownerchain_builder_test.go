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

package signalprocessing

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
)

// ============================================================================
// BR-SP-100: OwnerChain Traversal Tests
// DD-WORKFLOW-001 v1.8: Namespace, Kind, Name ONLY (no APIVersion/UID)
// Test File: test/unit/signalprocessing/ownerchain_builder_test.go
// ============================================================================

var _ = Describe("OwnerChain Builder", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		builder    *ownerchain.Builder
		scheme     *runtime.Scheme
		logger     logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()

		// Build scheme with all required types
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
	})

	// ============================================================================
	// HAPPY PATH TESTS (OC-HP-01 to OC-HP-04): 4 tests
	// ============================================================================

	Context("BR-SP-100: Happy Path - Standard Owner Chains", func() {

		// OC-HP-01: Pod → ReplicaSet → Deployment
		It("OC-HP-01: should build Pod → ReplicaSet → Deployment chain (2 entries)", func() {
			// Setup: Create Deployment → ReplicaSet → Pod ownership chain
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-deployment",
					Namespace: "production",
					UID:       types.UID("deploy-uid"),
				},
			}

			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-deployment-7d8f9c6b5",
					Namespace: "production",
					UID:       types.UID("rs-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "api-deployment",
							UID:        types.UID("deploy-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-deployment-7d8f9c6b5-abc12",
					Namespace: "production",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "api-deployment-7d8f9c6b5",
							UID:        types.UID("rs-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(deployment, replicaSet, pod).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "production", "Pod", "api-deployment-7d8f9c6b5-abc12")

			// Assert: Chain should be [ReplicaSet, Deployment] (owners only, source NOT included)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2))

			// First entry: ReplicaSet (immediate owner)
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[0].Name).To(Equal("api-deployment-7d8f9c6b5"))
			Expect(chain[0].Namespace).To(Equal("production"))

			// Second entry: Deployment (root owner)
			Expect(chain[1].Kind).To(Equal("Deployment"))
			Expect(chain[1].Name).To(Equal("api-deployment"))
			Expect(chain[1].Namespace).To(Equal("production"))
		})

		// OC-HP-02: Pod → StatefulSet
		It("OC-HP-02: should build Pod → StatefulSet chain (1 entry)", func() {
			statefulSet := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redis-cluster",
					Namespace: "data",
					UID:       types.UID("sts-uid"),
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redis-cluster-0",
					Namespace: "data",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "StatefulSet",
							Name:       "redis-cluster",
							UID:        types.UID("sts-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(statefulSet, pod).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "data", "Pod", "redis-cluster-0")

			// Assert: Chain should be [StatefulSet] (1 entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("StatefulSet"))
			Expect(chain[0].Name).To(Equal("redis-cluster"))
			Expect(chain[0].Namespace).To(Equal("data"))
		})

		// OC-HP-03: Pod → DaemonSet
		It("OC-HP-03: should build Pod → DaemonSet chain (1 entry)", func() {
			daemonSet := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fluentd",
					Namespace: "kube-system",
					UID:       types.UID("ds-uid"),
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fluentd-abc12",
					Namespace: "kube-system",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
							Name:       "fluentd",
							UID:        types.UID("ds-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(daemonSet, pod).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "kube-system", "Pod", "fluentd-abc12")

			// Assert: Chain should be [DaemonSet] (1 entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("DaemonSet"))
			Expect(chain[0].Name).To(Equal("fluentd"))
			Expect(chain[0].Namespace).To(Equal("kube-system"))
		})

		// OC-HP-04: Pod → Job → CronJob
		It("OC-HP-04: should build Pod → Job → CronJob chain (2 entries)", func() {
			cronJob := &batchv1.CronJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backup-job",
					Namespace: "default",
					UID:       types.UID("cj-uid"),
				},
			}

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backup-job-1234567890",
					Namespace: "default",
					UID:       types.UID("job-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1",
							Kind:       "CronJob",
							Name:       "backup-job",
							UID:        types.UID("cj-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "backup-job-1234567890-xyz",
					Namespace: "default",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "batch/v1",
							Kind:       "Job",
							Name:       "backup-job-1234567890",
							UID:        types.UID("job-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cronJob, job, pod).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "default", "Pod", "backup-job-1234567890-xyz")

			// Assert: Chain should be [Job, CronJob] (2 entries)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2))

			// First entry: Job
			Expect(chain[0].Kind).To(Equal("Job"))
			Expect(chain[0].Name).To(Equal("backup-job-1234567890"))
			Expect(chain[0].Namespace).To(Equal("default"))

			// Second entry: CronJob
			Expect(chain[1].Kind).To(Equal("CronJob"))
			Expect(chain[1].Name).To(Equal("backup-job"))
			Expect(chain[1].Namespace).To(Equal("default"))
		})
	})

	// ============================================================================
	// EDGE CASE TESTS (OC-EC-01 to OC-EC-04): 4 tests
	// ============================================================================

	Context("BR-SP-100: Edge Cases", func() {

		// OC-EC-01: Orphan Pod (no owner)
		It("OC-EC-01: should return empty chain for orphan Pod", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-pod",
					Namespace: "default",
					UID:       types.UID("pod-uid"),
					// No OwnerReferences - orphan pod
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(pod).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "default", "Pod", "orphan-pod")

			// Assert: Empty chain for orphan (no owners)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(BeEmpty())
		})

		// OC-EC-02: Node (cluster-scoped) - empty namespace
		It("OC-EC-02: should handle cluster-scoped resource with empty namespace", func() {
			// Node is cluster-scoped, has no owner
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-node-1",
					UID:  types.UID("node-uid"),
					// Nodes don't have owners typically
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(node).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute: Building chain for a Node (no namespace)
			chain, err := builder.Build(ctx, "", "Node", "worker-node-1")

			// Assert: Empty chain (nodes don't have owners)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(BeEmpty())
		})

		// OC-EC-03: Max depth reached (5 levels) - BR-SP-100
		It("OC-EC-03: should truncate chain at max depth (5 levels)", func() {
			// Create a deeply nested chain: Pod → RS1 → RS2 → RS3 → RS4 → RS5 → RS6 (6 levels)
			// Builder should stop at 5 levels per BR-SP-100

			// Build 7 ReplicaSets with chained ownership (6 levels above pod)
			var replicaSets []*appsv1.ReplicaSet
			for i := 6; i >= 1; i-- {
				rs := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rs-level-" + string(rune('0'+i)),
						Namespace: "default",
						UID:       types.UID("rs-uid-" + string(rune('0'+i))),
					},
				}
				// Add owner reference to next level (except the last one)
				if i < 6 {
					rs.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "rs-level-" + string(rune('0'+i+1)),
							UID:        types.UID("rs-uid-" + string(rune('0'+i+1))),
							Controller: boolPtr(true),
						},
					}
				}
				replicaSets = append(replicaSets, rs)
			}

			// Pod owned by rs-level-1
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deep-pod",
					Namespace: "default",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "rs-level-1",
							UID:        types.UID("rs-uid-1"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			// Build fake client with all objects
			objects := make([]client.Object, 0, len(replicaSets)+1)
			for _, rs := range replicaSets {
				objects = append(objects, rs)
			}
			objects = append(objects, pod)

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "default", "Pod", "deep-pod")

			// Assert: Chain should be truncated at 5 entries (MaxOwnerChainDepth)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(5)) // Max depth per BR-SP-100
		})

		// OC-EC-04: ReplicaSet without Deployment owner
		It("OC-EC-04: should return chain with ReplicaSet only when no Deployment owner", func() {
			// ReplicaSet with no owner (manually created, not by Deployment)
			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standalone-rs",
					Namespace: "staging",
					UID:       types.UID("rs-uid"),
					// No OwnerReferences - standalone ReplicaSet
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standalone-rs-abc12",
					Namespace: "staging",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "standalone-rs",
							UID:        types.UID("rs-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(replicaSet, pod).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "staging", "Pod", "standalone-rs-abc12")

			// Assert: Chain should be [ReplicaSet] only (1 entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[0].Name).To(Equal("standalone-rs"))
			Expect(chain[0].Namespace).To(Equal("staging"))
		})
	})

	// ============================================================================
	// ERROR HANDLING TESTS (OC-ER-01 to OC-ER-04): 4 tests
	// ============================================================================

	Context("BR-SP-100: Error Handling", func() {

		// OC-ER-01: K8s API timeout
		It("OC-ER-01: should return partial chain on K8s API timeout", func() {
			// Create a ReplicaSet owned by Deployment, but Deployment lookup will timeout
			// Per DD-WORKFLOW-001 v1.8: Owner is added to chain from ownerRef before fetch
			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-rs",
					Namespace: "production",
					UID:       types.UID("rs-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "api-deployment",
							UID:        types.UID("deploy-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-pod",
					Namespace: "production",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "api-rs",
							UID:        types.UID("rs-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			// Use interceptor to simulate timeout on Deployment fetch
			timeoutErr := errors.New("context deadline exceeded")
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(replicaSet, pod).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						// Timeout only on Deployment fetch
						if key.Name == "api-deployment" {
							return timeoutErr
						}
						return client.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "production", "Pod", "api-pod")

			// Assert: Chain includes RS and Deployment (added from RS's ownerRef)
			// The timeout happens when trying to fetch Deployment to continue traversal
			// Per DD-WORKFLOW-001 v1.8: Owner is added before fetch verification
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[0].Name).To(Equal("api-rs"))
			Expect(chain[1].Kind).To(Equal("Deployment"))
			Expect(chain[1].Name).To(Equal("api-deployment"))
		})

		// OC-ER-02: RBAC forbidden (403)
		It("OC-ER-02: should return partial chain on RBAC forbidden", func() {
			// Per DD-WORKFLOW-001 v1.8: Owner is added to chain from ownerRef before fetch
			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secure-rs",
					Namespace: "secure-ns",
					UID:       types.UID("rs-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "secure-deployment",
							UID:        types.UID("deploy-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secure-pod",
					Namespace: "secure-ns",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "secure-rs",
							UID:        types.UID("rs-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			// Use interceptor to simulate RBAC forbidden on Deployment fetch
			forbiddenErr := apierrors.NewForbidden(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"secure-deployment",
				errors.New("forbidden"),
			)

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(replicaSet, pod).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if key.Name == "secure-deployment" {
							return forbiddenErr
						}
						return client.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "secure-ns", "Pod", "secure-pod")

			// Assert: Chain includes RS and Deployment (added from RS's ownerRef)
			// The RBAC error happens when trying to fetch Deployment to continue
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[1].Kind).To(Equal("Deployment"))
		})

		// OC-ER-03: Resource not found
		It("OC-ER-03: should handle resource not found gracefully", func() {
			// Pod references a ReplicaSet that doesn't exist
			// Per DD-WORKFLOW-001 v1.8: Owner is added from Pod's ownerRef
			// The "not found" happens when trying to fetch RS to continue traversal
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphaned-ref-pod",
					Namespace: "default",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "deleted-rs", // This RS doesn't exist
							UID:        types.UID("deleted-rs-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(pod). // Note: ReplicaSet NOT added
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute
			chain, err := builder.Build(ctx, "default", "Pod", "orphaned-ref-pod")

			// Assert: Chain includes RS (from Pod's ownerRef)
			// The RS doesn't exist, but we still record it in the chain
			// This is useful for debugging orphaned pods
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[0].Name).To(Equal("deleted-rs"))
		})

		// OC-ER-04: Context cancelled
		It("OC-ER-04: should return current chain on context cancellation", func() {
			// Create full chain but cancel context during traversal
			// Per DD-WORKFLOW-001 v1.8: Owner is added from ownerRef before fetch
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancel-deployment",
					Namespace: "default",
					UID:       types.UID("deploy-uid"),
				},
			}

			replicaSet := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancel-rs",
					Namespace: "default",
					UID:       types.UID("rs-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       "cancel-deployment",
							UID:        types.UID("deploy-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancel-pod",
					Namespace: "default",
					UID:       types.UID("pod-uid"),
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       "cancel-rs",
							UID:        types.UID("rs-uid"),
							Controller: boolPtr(true),
						},
					},
				},
			}

			// Use interceptor to cancel context on Deployment fetch
			cancelledCtx, cancel := context.WithCancel(ctx)

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(deployment, replicaSet, pod).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if key.Name == "cancel-deployment" {
							cancel() // Cancel context before fetch
							// Small delay to ensure cancellation is processed
							<-time.After(10 * time.Millisecond)
							return ctx.Err()
						}
						return client.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			// Execute with cancellable context
			chain, err := builder.Build(cancelledCtx, "default", "Pod", "cancel-pod")

			// Assert: Chain includes RS and Deployment (both added from ownerRefs)
			// Context cancellation happens when trying to continue past Deployment
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[1].Kind).To(Equal("Deployment"))
		})
	})

	// ============================================================================
	// CONSTRUCTOR TEST
	// ============================================================================

	Context("NewBuilder Constructor", func() {
		It("should create builder with valid dependencies", func() {
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, logger)

			Expect(builder).NotTo(BeNil())
		})
	})
})

// Note: boolPtr helper is defined in enricher_test.go (same package)
