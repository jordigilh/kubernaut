package signalprocessing

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
)

// BR-SP-100: OwnerChain Traversal
// DD-WORKFLOW-001 v1.8: SignalProcessing traverses ownerReferences to build chain
var _ = Describe("BR-SP-100: OwnerChain Builder", func() {
	var (
		ctx     context.Context
		scheme  *runtime.Scheme
		builder *ownerchain.Builder
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// TC-OC-001 to TC-OC-006: OwnerChain Scenarios
	// ========================================
	DescribeTable("should build owner chain for various resource types",
		func(setupFn func() []runtime.Object, namespace, kind, name string, expectedLen int, expectedKinds []string) {
			objects := setupFn()
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, ctrl.Log.WithName("test"))

			chain, err := builder.Build(ctx, namespace, kind, name)

			Expect(err).ToNot(HaveOccurred())
			Expect(chain).To(HaveLen(expectedLen))

			// Verify each entry's Kind matches expected
			for i, expectedKind := range expectedKinds {
				Expect(chain[i].Kind).To(Equal(expectedKind), "Entry %d should be %s", i, expectedKind)
			}
		},
		Entry("TC-OC-001: Pod→RS→Deployment chain",
			func() []runtime.Object {
				return []runtime.Object{
					createPodWithOwner("web-pod", "default", "ReplicaSet", "web-rs"),
					createReplicaSetWithOwner("web-rs", "default", "Deployment", "web-deploy"),
					createDeployment("web-deploy", "default"),
				}
			},
			"default", "Pod", "web-pod", 3, []string{"Pod", "ReplicaSet", "Deployment"}),

		Entry("TC-OC-002: Pod→StatefulSet chain",
			func() []runtime.Object {
				return []runtime.Object{
					createPodWithOwner("db-0", "default", "StatefulSet", "db"),
					createStatefulSet("db", "default"),
				}
			},
			"default", "Pod", "db-0", 2, []string{"Pod", "StatefulSet"}),

		Entry("TC-OC-003: Pod→DaemonSet chain",
			func() []runtime.Object {
				return []runtime.Object{
					createPodWithOwner("fluentd-xyz", "kube-system", "DaemonSet", "fluentd"),
					createDaemonSet("fluentd", "kube-system"),
				}
			},
			"kube-system", "Pod", "fluentd-xyz", 2, []string{"Pod", "DaemonSet"}),

		Entry("TC-OC-004: Node (cluster-scoped)",
			func() []runtime.Object {
				return []runtime.Object{
					createNode("worker-1"),
				}
			},
			"", "Node", "worker-1", 1, []string{"Node"}),

		Entry("TC-OC-005: Orphan Pod (no owner)",
			func() []runtime.Object {
				return []runtime.Object{
					createOrphanPod("orphan-pod", "default"),
				}
			},
			"default", "Pod", "orphan-pod", 1, []string{"Pod"}),
	)

	// ========================================
	// TC-OC-006: Max Depth Protection
	// ========================================
	It("TC-OC-006: should limit chain to 10 levels", func() {
		// Create a chain that would be 15 levels deep
		// Builder should cap at 10
		objects := createDeepChain(15)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithRuntimeObjects(objects...).
			Build()

		builder = ownerchain.NewBuilder(fakeClient, ctrl.Log.WithName("test"))

		chain, err := builder.Build(ctx, "default", "Pod", "deep-pod-0")

		Expect(err).ToNot(HaveOccurred())
		Expect(len(chain)).To(BeNumerically("<=", 10), "Chain should be capped at 10 levels")
	})

	// ========================================
	// Edge Cases
	// ========================================
	Describe("Edge Cases", func() {
		It("should handle non-existent resource gracefully", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, ctrl.Log.WithName("test"))

			chain, err := builder.Build(ctx, "default", "Pod", "non-existent")

			// Should return single entry (the requested resource) even if not found
			// This is graceful degradation - we don't fail if resource doesn't exist
			Expect(err).ToNot(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("Pod"))
			Expect(chain[0].Name).To(Equal("non-existent"))
		})

		It("should handle Pod with multiple owners (pick controller)", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-owner-pod",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "ConfigMap",
							Name:       "config",
							Controller: boolPtr(false), // Not controller
						},
						{
							Kind:       "ReplicaSet",
							Name:       "web-rs",
							Controller: boolPtr(true), // This is the controller
						},
					},
				},
			}
			rs := createReplicaSetWithOwner("web-rs", "default", "Deployment", "web-deploy")
			deploy := createDeployment("web-deploy", "default")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(pod, rs, deploy).
				Build()

			builder = ownerchain.NewBuilder(fakeClient, ctrl.Log.WithName("test"))

			chain, err := builder.Build(ctx, "default", "Pod", "multi-owner-pod")

			Expect(err).ToNot(HaveOccurred())
			Expect(chain).To(HaveLen(3))
			Expect(chain[0].Kind).To(Equal("Pod"))
			Expect(chain[1].Kind).To(Equal("ReplicaSet")) // Should pick the controller owner
			Expect(chain[2].Kind).To(Equal("Deployment"))
		})
	})
})

// Helper functions to create test objects
func createPodWithOwner(name, namespace, ownerKind, ownerName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       ownerKind,
					Name:       ownerName,
					Controller: boolPtr(true),
				},
			},
		},
	}
}

func createOrphanPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func createReplicaSetWithOwner(name, namespace, ownerKind, ownerName string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       ownerKind,
					Name:       ownerName,
					Controller: boolPtr(true),
				},
			},
		},
	}
}

func createDeployment(name, namespace string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func createStatefulSet(name, namespace string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func createDaemonSet(name, namespace string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func createNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func createDeepChain(depth int) []runtime.Object {
	// Creates a chain of ConfigMaps (for simplicity) with ownership
	// Pod → ConfigMap0 → ConfigMap1 → ... → ConfigMapN
	var objects []runtime.Object

	// Start with a pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deep-pod-0",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "ConfigMap",
					Name:       "cm-1",
					Controller: boolPtr(true),
				},
			},
		},
	}
	objects = append(objects, pod)

	// Create ConfigMap chain
	for i := 1; i < depth; i++ {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cm-" + string(rune('0'+i)),
				Namespace: "default",
			},
		}
		if i < depth-1 {
			cm.OwnerReferences = []metav1.OwnerReference{
				{
					Kind:       "ConfigMap",
					Name:       "cm-" + string(rune('0'+i+1)),
					Controller: boolPtr(true),
				},
			}
		}
		objects = append(objects, cm)
	}

	return objects
}

func boolPtr(b bool) *bool {
	return &b
}
