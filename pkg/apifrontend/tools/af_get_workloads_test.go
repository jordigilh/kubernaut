package tools_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newUnstructuredDeployment(ns, name string, desired, ready, available int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"replicas": desired,
			},
			"status": map[string]interface{}{
				"readyReplicas":     ready,
				"availableReplicas": available,
			},
		},
	}
}

func newUnstructuredStatefulSet(ns, name string, desired, ready int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"replicas": desired,
			},
			"status": map[string]interface{}{
				"readyReplicas": ready,
			},
		},
	}
}

func newUnstructuredDaemonSet(ns, name string, desired, ready, available int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "DaemonSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"status": map[string]interface{}{
				"desiredNumberScheduled": desired,
				"numberReady":            ready,
				"numberAvailable":        available,
			},
		},
	}
}

func newUnstructuredJob(ns, name string, completions, succeeded int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"completions": completions,
			},
			"status": map[string]interface{}{
				"succeeded": succeeded,
			},
		},
	}
}

func newUnstructuredCronJob(ns, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "CronJob",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"schedule": "*/5 * * * *",
			},
		},
	}
}

var allWorkloadGVRs = map[schema.GroupVersionResource]string{
	{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
	{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
	{Group: "apps", Version: "v1", Resource: "daemonsets"}:   "DaemonSetList",
	{Group: "batch", Version: "v1", Resource: "jobs"}:        "JobList",
	{Group: "batch", Version: "v1", Resource: "cronjobs"}:    "CronJobList",
}

var _ = Describe("af_get_workloads", func() {

	It("UT-AF-052-020: happy path returns Deployment and StatefulSet summaries", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs,
			newUnstructuredDeployment("prod", "web", 3, 3, 3),
			newUnstructuredStatefulSet("prod", "db", 2, 2),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Workloads[0].Kind).To(Equal("Deployment"))
		Expect(result.Workloads[0].Name).To(Equal("web"))
		Expect(result.Workloads[0].Replicas.Desired).To(Equal(int64(3)))
		Expect(result.Workloads[1].Kind).To(Equal("StatefulSet"))
	})

	It("UT-AF-052-021: empty namespace rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs)
		_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: ""})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-022: invalid namespace rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs)
		_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "../etc"})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-023: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleGetWorkloads(context.Background(), nil, tools.GetWorkloadsArgs{Namespace: "default"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-024: name filter returns only matching workload", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs,
			newUnstructuredDeployment("prod", "web", 3, 3, 3),
			newUnstructuredDeployment("prod", "api", 2, 1, 1),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "prod", Name: "web"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Workloads[0].Name).To(Equal("web"))
	})

	It("UT-AF-052-025: invalid name rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs)
		_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{
			Namespace: "default",
			Name:      "INVALID_NAME!!",
		})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-026: empty namespace returns empty list", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "empty-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-052-028: returns DaemonSet workloads", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs,
			newUnstructuredDaemonSet("kube-system", "fluentd", 3, 3, 3),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "kube-system"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Workloads[0].Kind).To(Equal("DaemonSet"))
		Expect(result.Workloads[0].Name).To(Equal("fluentd"))
		Expect(result.Workloads[0].Replicas.Desired).To(Equal(int64(3)))
		Expect(result.Workloads[0].Replicas.Ready).To(Equal(int64(3)))
		Expect(result.Workloads[0].Replicas.Available).To(Equal(int64(3)))
	})

	It("UT-AF-052-029: returns Job workloads", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs,
			newUnstructuredJob("batch-ns", "migrate-db", 1, 0),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "batch-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Workloads[0].Kind).To(Equal("Job"))
		Expect(result.Workloads[0].Name).To(Equal("migrate-db"))
		Expect(result.Workloads[0].Replicas.Desired).To(Equal(int64(1)))
		Expect(result.Workloads[0].Replicas.Ready).To(Equal(int64(0)))
	})

	It("UT-AF-052-030: returns CronJob workloads", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs,
			newUnstructuredCronJob("batch-ns", "nightly-backup"),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "batch-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Workloads[0].Kind).To(Equal("CronJob"))
		Expect(result.Workloads[0].Name).To(Equal("nightly-backup"))
	})

	It("UT-AF-052-031: mixed workload types returned together", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs,
			newUnstructuredDeployment("prod", "web", 3, 3, 3),
			newUnstructuredStatefulSet("prod", "db", 2, 2),
			newUnstructuredDaemonSet("prod", "log-collector", 5, 5, 5),
			newUnstructuredJob("prod", "seed-data", 1, 1),
			newUnstructuredCronJob("prod", "report-gen"),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(5))

		kinds := make(map[string]bool)
		for _, w := range result.Workloads {
			kinds[w.Kind] = true
		}
		Expect(kinds).To(HaveKey("Deployment"))
		Expect(kinds).To(HaveKey("StatefulSet"))
		Expect(kinds).To(HaveKey("DaemonSet"))
		Expect(kinds).To(HaveKey("Job"))
		Expect(kinds).To(HaveKey("CronJob"))
	})

	It("UT-AF-052-027: concurrent calls are safe", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allWorkloadGVRs,
			newUnstructuredDeployment("test", "svc", 1, 1, 1),
		)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "test"})
				Expect(err).NotTo(HaveOccurred())
			}()
		}
		wg.Wait()
	})
})
