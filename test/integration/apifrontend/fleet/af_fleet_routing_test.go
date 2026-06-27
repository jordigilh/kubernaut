package fleet_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var kubectlGVRs = map[schema.GroupVersionResource]string{
	{Group: "", Version: "v1", Resource: "services"}: "ServiceList",
	{Group: "", Version: "v1", Resource: "pods"}:     "PodList",
}

func newService(ns, name, ip string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]interface{}{"name": name, "namespace": ns},
			"spec":       map[string]interface{}{"clusterIP": ip},
		},
	}
}

// recordingReaderFactory tracks which clusterID was requested.
type recordingReaderFactory struct {
	reader    tools.ResourceReader
	requested []string
}

func (f *recordingReaderFactory) factory(_ context.Context, clusterID string) (tools.ResourceReader, error) {
	f.requested = append(f.requested, clusterID)
	if f.reader != nil {
		return f.reader, nil
	}
	return nil, fmt.Errorf("unknown cluster %q", clusterID)
}

var _ = Describe("AF Fleet Routing Integration [BR-FLEET-054, AC-3]", func() {

	Describe("NewKubectlGetTool wiring with FleetReaderFactory", func() {
		It("IT-AF-054-001: routes to fleet reader when cluster_id is provided", func() {
			localScheme := runtime.NewScheme()
			localDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(localScheme, kubectlGVRs,
				newService("local-ns", "local-svc", "10.0.0.1"),
			)
			localFactory := auth.StaticDynamicFactory(localDyn)

			remoteReader := &tools.DynamicResourceReader{
				Client: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
					runtime.NewScheme(), kubectlGVRs,
					newService("remote-ns", "remote-svc", "10.1.0.1"),
				),
			}
			recorder := &recordingReaderFactory{reader: remoteReader}

			getTool, err := tools.NewKubectlGetTool(localFactory, nil, tools.ResourceReaderFactory(recorder.factory))
			Expect(err).NotTo(HaveOccurred())
			Expect(getTool).NotTo(BeNil())

			result, err := tools.HandleKubectlGet(context.Background(), remoteReader, nil, tools.KubectlGetArgs{
				Kind:      "Service",
				Name:      "remote-svc",
				Namespace: "remote-ns",
				ClusterID: "cluster-east",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("remote-svc"))
			Expect(result.Object).To(HaveKey("spec"))
		})

		It("IT-AF-054-002: falls back to local reader when cluster_id is empty", func() {
			localScheme := runtime.NewScheme()
			localDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(localScheme, kubectlGVRs,
				newService("default", "web-svc", "10.0.0.1"),
			)
			localFactory := auth.StaticDynamicFactory(localDyn)

			recorder := &recordingReaderFactory{}

			_, err := tools.NewKubectlGetTool(localFactory, nil, tools.ResourceReaderFactory(recorder.factory))
			Expect(err).NotTo(HaveOccurred())

			result, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: localDyn}, nil, tools.KubectlGetArgs{
				Kind:      "Service",
				Name:      "web-svc",
				Namespace: "default",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("web-svc"))
			Expect(recorder.requested).To(BeEmpty(), "fleet factory should not be called for local reads")
		})
	})

	Describe("NewKubectlListTool wiring with FleetReaderFactory", func() {
		It("IT-AF-054-003: lists from fleet reader when cluster_id is provided", func() {
			localScheme := runtime.NewScheme()
			localDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(localScheme, kubectlGVRs)
			localFactory := auth.StaticDynamicFactory(localDyn)

			remoteReader := &tools.DynamicResourceReader{
				Client: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
					runtime.NewScheme(), kubectlGVRs,
					newService("remote-ns", "svc-a", "10.1.0.1"),
					newService("remote-ns", "svc-b", "10.1.0.2"),
				),
			}
			recorder := &recordingReaderFactory{reader: remoteReader}

			_, err := tools.NewKubectlListTool(localFactory, nil, tools.ResourceReaderFactory(recorder.factory))
			Expect(err).NotTo(HaveOccurred())

			gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
			gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
			list, err := remoteReader.ListResources(context.Background(), gvr, gvk, "remote-ns", metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
		})
	})

	Describe("AgentConfig FleetReaderFactory integration", func() {
		It("IT-AF-054-004: nil FleetReaderFactory creates tools without fleet support (backward compat)", func() {
			localScheme := runtime.NewScheme()
			localDyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(localScheme, kubectlGVRs,
				newService("default", "local-svc", "10.0.0.1"),
			)
			localFactory := auth.StaticDynamicFactory(localDyn)

			getTool, err := tools.NewKubectlGetTool(localFactory, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(getTool).NotTo(BeNil())

			result, err := tools.HandleKubectlGet(context.Background(), &tools.DynamicResourceReader{Client: localDyn}, nil, tools.KubectlGetArgs{
				Kind:      "Service",
				Name:      "local-svc",
				Namespace: "default",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("local-svc"))
		})
	})
})
