package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newUnstructuredService(ns, name, clusterIP string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"clusterIP": clusterIP,
				"type":      "ClusterIP",
			},
		},
	}
}

func newUnstructuredSecret(ns, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"type": "Opaque",
			"data": map[string]interface{}{
				"password": "c2VjcmV0cGFzcw==",
				"token":    "dG9rZW52YWx1ZQ==",
			},
		},
	}
}

func newUnstructuredEndpoints(ns, name string, addresses []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Endpoints",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"subsets": []interface{}{
				map[string]interface{}{
					"addresses": addresses,
				},
			},
		},
	}
}

var kubectlGVRs = map[schema.GroupVersionResource]string{
	{Group: "", Version: "v1", Resource: "services"}:  "ServiceList",
	{Group: "", Version: "v1", Resource: "secrets"}:   "SecretList",
	{Group: "", Version: "v1", Resource: "endpoints"}: "EndpointsList",
	{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
}

var _ = Describe("kubectl_get", func() {
	It("UT-AF-1230-001: returns a single resource by kind/name/namespace", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
			newUnstructuredService("prod", "web-svc", "10.0.0.1"),
		)

		result, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "Service",
			Name:      "web-svc",
			Namespace: "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Kind).To(Equal("Service"))
		Expect(result.Name).To(Equal("web-svc"))
		Expect(result.Namespace).To(Equal("prod"))
		Expect(result.Object).To(HaveKey("spec"))
	})

	It("UT-AF-1230-002: returns not found for nonexistent resource", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		_, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "Service",
			Name:      "ghost",
			Namespace: "prod",
		})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("not found")))
	})

	It("UT-AF-1230-003: redacts Secret .data field", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
			newUnstructuredSecret("prod", "db-creds"),
		)

		result, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "Secret",
			Name:      "db-creds",
			Namespace: "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Kind).To(Equal("Secret"))
		data, exists := result.Object["data"]
		if exists {
			dataMap, ok := data.(map[string]interface{})
			Expect(ok).To(BeTrue())
			for _, v := range dataMap {
				Expect(v).To(Equal("REDACTED"))
			}
		}
	})

	It("UT-AF-1230-004: rejects empty kind", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		_, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "",
			Name:      "web-svc",
			Namespace: "prod",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-1230-005: rejects empty name", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		_, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "Service",
			Name:      "",
			Namespace: "prod",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-1230-006: rejects invalid namespace", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		_, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "Service",
			Name:      "web-svc",
			Namespace: "../etc",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-1230-007: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleKubectlGet(context.Background(), nil, nil, tools.KubectlGetArgs{
			Kind:      "Service",
			Name:      "web-svc",
			Namespace: "prod",
		})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-1230-008a: redacts Secret .stringData field", func() {
		secret := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "inline-creds",
					"namespace": "prod",
				},
				"type": "Opaque",
				"stringData": map[string]interface{}{
					"username": "admin",
					"password": "hunter2",
				},
			},
		}
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs, secret)

		result, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "Secret",
			Name:      "inline-creds",
			Namespace: "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		sd, exists := result.Object["stringData"]
		if exists {
			sdMap, ok := sd.(map[string]interface{})
			Expect(ok).To(BeTrue())
			for _, v := range sdMap {
				Expect(v).To(Equal("REDACTED"))
			}
		}
	})

	It("UT-AF-1230-009: unknown Kind with nil mapper returns error", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		_, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "FooBarBaz",
			Name:      "test",
			Namespace: "prod",
		})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-1230-008: returns Endpoints resource", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
			newUnstructuredEndpoints("prod", "web-svc", []interface{}{
				map[string]interface{}{"ip": "10.244.0.5"},
			}),
		)

		result, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: client}, nil, tools.KubectlGetArgs{
			Kind:      "Endpoints",
			Name:      "web-svc",
			Namespace: "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Kind).To(Equal("Endpoints"))
		Expect(result.Object).To(HaveKey("subsets"))
	})
})
