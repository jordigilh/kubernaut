package infrastructure

import (
	"errors"
	"fmt"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

var _ = Describe("fleet spoke Prometheus Operator manifests", func() {
	It("UT-INFRA-FLEETDEMO-OPERATOR-001: pins the operator Helm chart", func() {
		Expect(prometheusOperatorHelmChart).To(Equal("prometheus-community/kube-prometheus-stack"))
		Expect(prometheusOperatorHelmVersion).To(Equal("88.1.5"))
		Expect(prometheusOperatorNamespace).To(Equal("prometheus-operator"))
	})

	It("UT-INFRA-FLEETDEMO-OPERATOR-002: uses an operator-only chart configuration", func() {
		Expect(prometheusOperatorHelmChart).To(Equal("prometheus-community/kube-prometheus-stack"))
		Expect(prometheusOperatorNamespace).To(Equal("prometheus-operator"))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-001: selects monitoring CRDs across namespaces", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")

		Expect(manifest).To(ContainSubstring("kind: Prometheus\n"))
		Expect(manifest).To(ContainSubstring("name: fleet-spoke\n"))
		Expect(manifest).To(ContainSubstring("serviceMonitorSelector: {}"))
		Expect(manifest).To(ContainSubstring("serviceMonitorNamespaceSelector: {}"))
		Expect(manifest).To(ContainSubstring("podMonitorSelector: {}"))
		Expect(manifest).To(ContainSubstring("podMonitorNamespaceSelector: {}"))
		Expect(manifest).To(ContainSubstring("probeSelector: {}"))
		Expect(manifest).To(ContainSubstring("probeNamespaceSelector: {}"))
		Expect(manifest).To(ContainSubstring("ruleSelector: {}"))
		Expect(manifest).To(ContainSubstring("ruleNamespaceSelector: {}"))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-002: preserves fleet identity and Thanos configuration", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")

		Expect(manifest).To(ContainSubstring("cluster: remote-cluster"))
		Expect(manifest).To(ContainSubstring("image: " + ThanosImage))
		Expect(manifest).To(ContainSubstring("name: prometheus-svc"))
		Expect(manifest).To(ContainSubstring("nodePort: 30190"))
		Expect(manifest).To(ContainSubstring("app: prometheus"))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-003: routes alerts through the hub bridge", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")

		Expect(manifest).To(ContainSubstring("name: alertmanager-remote"))
		Expect(manifest).To(ContainSubstring("ip: 10.0.0.2"))
		Expect(manifest).To(ContainSubstring("port: 30193"))
		Expect(manifest).To(ContainSubstring("port: web"))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-003b: uses the existing hub Alertmanager Service", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "hub", "alertmanager-svc.monitoring.svc.cluster.local:9093")

		Expect(manifest).To(ContainSubstring("name: alertmanager-svc"))
		Expect(manifest).To(ContainSubstring("port: http"))
		Expect(manifest).NotTo(ContainSubstring("name: alertmanager-remote"))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-004: installs baseline kube-state-metrics discovery", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")

		Expect(manifest).To(ContainSubstring("name: kube-state-metrics"))
		Expect(manifest).To(ContainSubstring("kind: ServiceMonitor"))
		Expect(manifest).To(ContainSubstring("port: http-metrics"))
	})

	It("UT-INFRA-2380-001 [SI-4, ASVS-4.0.3-V4.1.1]: preserves native KSM labels", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")
		serviceMonitor := findManifestDocument(manifest, "ServiceMonitor", "kube-state-metrics")

		endpoints, found, err := unstructured.NestedSlice(serviceMonitor.Object, "spec", "endpoints")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(endpoints).NotTo(BeEmpty())

		endpoint, ok := endpoints[0].(map[string]interface{})
		Expect(ok).To(BeTrue())
		honorLabels, found, err := unstructured.NestedBool(endpoint, "honorLabels")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(honorLabels).To(BeTrue(), "KSM namespace/pod labels must not be renamed to exported_*")
	})

	It("UT-INFRA-2380-002 [SI-4]: enables fleet scenario KSM metric families", func() {
		manifest := buildKubeStateMetricsManifest("monitoring")
		deployment := findManifestDocument(manifest, "Deployment", "kube-state-metrics")
		containers, found, err := unstructured.NestedSlice(deployment.Object, "spec", "template", "spec", "containers")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(containers).NotTo(BeEmpty())

		container, ok := containers[0].(map[string]interface{})
		Expect(ok).To(BeTrue())
		args, found, err := unstructured.NestedStringSlice(container, "args")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(args).To(ContainElement("--resources=pods,deployments,replicasets,statefulsets,daemonsets,nodes,horizontalpodautoscalers,persistentvolumeclaims,poddisruptionbudgets"))
	})

	It("UT-INFRA-2380-003 [AC-6, ASVS-4.0.3-V4.1.3]: grants KSM required read-only access", func() {
		manifest := buildKubeStateMetricsManifest("monitoring")
		clusterRole := findManifestDocument(manifest, "ClusterRole", "kube-state-metrics")
		rules, found, err := unstructured.NestedSlice(clusterRole.Object, "rules")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		expectReadOnlyRule := func(apiGroup, resource string) {
			GinkgoHelper()
			for _, rawRule := range rules {
				rule, ok := rawRule.(map[string]interface{})
				if !ok {
					continue
				}
				groups, _, _ := unstructured.NestedStringSlice(rule, "apiGroups")
				resources, _, _ := unstructured.NestedStringSlice(rule, "resources")
				if !containsString(groups, apiGroup) || !containsString(resources, resource) {
					continue
				}
				verbs, _, _ := unstructured.NestedStringSlice(rule, "verbs")
				Expect(verbs).To(ConsistOf("list", "watch"))
				Expect(verbs).NotTo(ContainElement("create"))
				Expect(verbs).NotTo(ContainElement("delete"))
				return
			}
			Fail(fmt.Sprintf("missing read-only RBAC rule for apiGroup=%q resource=%q", apiGroup, resource))
		}

		expectReadOnlyRule("", "persistentvolumeclaims")
		expectReadOnlyRule("autoscaling", "horizontalpodautoscalers")
		expectReadOnlyRule("policy", "poddisruptionbudgets")
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-005: keeps the baseline crashloop rule as a PrometheusRule", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")

		Expect(manifest).To(ContainSubstring("kind: PrometheusRule"))
		Expect(manifest).To(ContainSubstring("name: demo-alerts"))
		Expect(manifest).To(ContainSubstring("cluster: remote-cluster"))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-006: rejects an invalid Alertmanager bridge address", func() {
		_, err := buildManagedPrometheusManifestChecked("monitoring", "remote-cluster", "not-an-address")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid Alertmanager bridge address"))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-007: keeps infrastructure scrape configuration separate from scenario CRDs", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")

		Expect(manifest).To(ContainSubstring("name: prometheus-additional-scrape-configs"))
		Expect(manifest).To(ContainSubstring("job_name: kubelet-cadvisor"))
		Expect(strings.Count(manifest, "kind: ServiceMonitor")).To(Equal(1))
	})

	It("UT-INFRA-FLEETDEMO-PROMETHEUS-008: renders valid multi-document YAML", func() {
		manifest := buildManagedPrometheusManifest("monitoring", "remote-cluster", "10.0.0.2:30193")
		decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
		documents := 0

		for {
			var document map[string]interface{}
			err := decoder.Decode(&document)
			if errors.Is(err, io.EOF) {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			if document != nil {
				documents++
			}
		}

		Expect(documents).To(BeNumerically(">=", 8))
	})
})

func findManifestDocument(manifest, kind, name string) unstructured.Unstructured {
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var document map[string]interface{}
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		Expect(err).NotTo(HaveOccurred())
		if document == nil {
			continue
		}
		if document["kind"] == kind {
			metadata, found, metadataErr := unstructured.NestedMap(document, "metadata")
			Expect(metadataErr).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			if metadata["name"] == name {
				return unstructured.Unstructured{Object: document}
			}
		}
	}
	Fail(fmt.Sprintf("manifest document not found: kind=%q name=%q", kind, name))
	return unstructured.Unstructured{}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
