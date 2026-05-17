package integration_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const itNamespace = "it-apifrontend"

var (
	rrGVR  = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
	rarGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationapprovalrequests"}
)

// seedK8sFixtures creates the namespace and K8s resources required by IT specs.
// Runs in Phase 2 (all nodes) since itDynClient is only available after Phase 2 wiring.
func seedK8sFixtures(ctx context.Context, client dynamic.Interface) {
	By("IT-ENV-003: Seeding K8s fixtures in envtest")

	createNamespace(ctx, client, itNamespace)

	// Deployment for af_get_workloads / af_resolve_owner
	createDeployment(ctx, client, itNamespace, "test-deploy", map[string]string{"app": "test-deploy"})

	// ReplicaSet owned by the Deployment (for af_resolve_owner chain)
	createReplicaSet(ctx, client, itNamespace, "test-deploy-rs-abc12", "test-deploy",
		map[string]string{"app": "test-deploy", "pod-template-hash": "abc12"})

	// Pod owned by the ReplicaSet (for af_get_pods / af_resolve_owner)
	createPod(ctx, client, itNamespace, "test-deploy-rs-abc12-pod1", "test-deploy-rs-abc12",
		map[string]string{"app": "test-deploy", "pod-template-hash": "abc12"})

	// Event for af_list_events
	createEvent(ctx, client, itNamespace, "test-deploy-oom", "OOMKilled", "Container exceeded memory limit",
		"Pod", "test-deploy-rs-abc12-pod1")

	// RemediationRequest for kubernaut_list_remediations / get_remediation / check_existing_rr
	createRemediationRequest(ctx, client, itNamespace, "test-rr-001", "Deployment", "test-deploy")

	// RemediationApprovalRequest for kubernaut_approve
	createRemediationApprovalRequest(ctx, client, itNamespace, "test-rar-001", "test-rr-001")

	_, _ = fmt.Fprintln(GinkgoWriter, "K8s fixtures seeded successfully")
}

func createNamespace(ctx context.Context, client dynamic.Interface, name string) {
	ns := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"name": name},
	}}
	nsGVR := schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
	_, err := client.Resource(nsGVR).Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "namespace %q may already exist: %v\n", name, err)
	}
}

func createDeployment(ctx context.Context, client dynamic.Interface, ns, name string, labels map[string]string) {
	deploy := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels":    toStringMap(labels),
			"uid":       "deploy-uid-" + name,
		},
		"spec": map[string]any{
			"replicas": int64(2),
			"selector": map[string]any{
				"matchLabels": toStringMap(labels),
			},
			"template": map[string]any{
				"metadata": map[string]any{"labels": toStringMap(labels)},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "app", "image": "nginx:latest"},
					},
				},
			},
		},
	}}
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	created, err := client.Resource(deployGVR).Namespace(ns).Create(ctx, deploy, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "deploy fixture creation")
	_, _ = fmt.Fprintf(GinkgoWriter, "  Deployment %s/%s created (UID: %s)\n", ns, name, created.GetUID())
}

func createReplicaSet(ctx context.Context, client dynamic.Interface, ns, name, ownerDeployName string, labels map[string]string) {
	// Look up the Deployment UID for the ownerReference
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	deploy, err := client.Resource(deployGVR).Namespace(ns).Get(ctx, ownerDeployName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred(), "lookup owner deployment for replicaset")

	isController := true
	rs := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels":    toStringMap(labels),
			"ownerReferences": []any{
				map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       ownerDeployName,
					"uid":        string(deploy.GetUID()),
					"controller": isController,
				},
			},
		},
		"spec": map[string]any{
			"replicas": int64(2),
			"selector": map[string]any{
				"matchLabels": toStringMap(labels),
			},
			"template": map[string]any{
				"metadata": map[string]any{"labels": toStringMap(labels)},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "app", "image": "nginx:latest"},
					},
				},
			},
		},
	}}
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	created, err := client.Resource(rsGVR).Namespace(ns).Create(ctx, rs, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "replicaset fixture creation")
	_, _ = fmt.Fprintf(GinkgoWriter, "  ReplicaSet %s/%s created (UID: %s)\n", ns, name, created.GetUID())
}

func createPod(ctx context.Context, client dynamic.Interface, ns, name, ownerRSName string, labels map[string]string) {
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	rs, err := client.Resource(rsGVR).Namespace(ns).Get(ctx, ownerRSName, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred(), "lookup owner replicaset for pod")

	isController := true
	pod := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels":    toStringMap(labels),
			"ownerReferences": []any{
				map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "ReplicaSet",
					"name":       ownerRSName,
					"uid":        string(rs.GetUID()),
					"controller": isController,
				},
			},
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "app", "image": "nginx:latest"},
			},
		},
	}}
	podGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	_, err = client.Resource(podGVR).Namespace(ns).Create(ctx, pod, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "pod fixture creation")
}

func createEvent(ctx context.Context, client dynamic.Interface, ns, name, reason, message, involvedKind, involvedName string) {
	event := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Event",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
		},
		"involvedObject": map[string]any{
			"kind":      involvedKind,
			"name":      involvedName,
			"namespace": ns,
		},
		"reason":  reason,
		"message": message,
		"type":    "Warning",
	}}
	eventGVR := schema.GroupVersionResource{Version: "v1", Resource: "events"}
	_, err := client.Resource(eventGVR).Namespace(ns).Create(ctx, event, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "event fixture creation")
}

func createRemediationRequest(ctx context.Context, client dynamic.Interface, ns, name, targetKind, targetName string) {
	now := time.Now().UTC().Format(time.RFC3339)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(name+targetKind+targetName)))

	rr := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubernaut.ai/v1alpha1",
		"kind":       "RemediationRequest",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels": map[string]any{
				"kubernaut.ai/target-kind": targetKind,
				"kubernaut.ai/target-name": targetName,
			},
		},
		"spec": map[string]any{
			"signalName":        "it-signal-" + name,
			"signalType":        "alert",
			"signalFingerprint": fingerprint,
			"severity":          "medium",
			"firingTime":        now,
			"receivedTime":      now,
			"targetType":        "kubernetes",
			"targetResource": map[string]any{
				"kind": targetKind,
				"name": targetName,
			},
		},
	}}
	_, err := client.Resource(rrGVR).Namespace(ns).Create(ctx, rr, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "RR fixture creation")
}

func createRemediationApprovalRequest(ctx context.Context, client dynamic.Interface, ns, name, rrRef string) {
	now := time.Now().UTC().Format(time.RFC3339)

	rar := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubernaut.ai/v1alpha1",
		"kind":       "RemediationApprovalRequest",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]any{
			"remediationRequestRef": map[string]any{
				"name":      rrRef,
				"namespace": ns,
			},
			"aiAnalysisRef": map[string]any{
				"name": "it-analysis-" + name,
			},
			"confidence":      0.65,
			"confidenceLevel": "medium",
			"investigationSummary": "Integration test: OOMKilled pod detected, " +
				"restart workflow recommended with medium confidence.",
			"reason":              "Confidence below auto-approve threshold",
			"whyApprovalRequired": "AI confidence 0.65 is below the 0.80 auto-approve threshold",
			"requiredBy":          now,
			"recommendedActions": []any{
				map[string]any{
					"action":    "RestartPod",
					"rationale": "Pod exceeded memory limit; restart clears OOM state",
				},
			},
			"recommendedWorkflow": map[string]any{
				"workflowId":      "wf-restart-pod-v1",
				"version":         "1.0.0",
				"executionBundle": "ghcr.io/jordigilh/kubernaut/bundles/restart-pod@sha256:abc123",
				"rationale":       "Standard restart for OOMKilled pods",
			},
		},
	}}
	_, err := client.Resource(rarGVR).Namespace(ns).Create(ctx, rar, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "RAR fixture creation")
}

func toStringMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
