package tools_test

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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// stubResourceReader records calls and returns preconfigured results.
type stubResourceReader struct {
	getResult  *unstructured.Unstructured
	listResult *unstructured.UnstructuredList
	getErr     error
	listErr    error
	lastGVR    schema.GroupVersionResource
	lastGVK    schema.GroupVersionKind
	lastNS     string
	lastName   string
}

func (s *stubResourceReader) GetResource(_ context.Context, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, ns, name string) (*unstructured.Unstructured, error) {
	s.lastGVR = gvr
	s.lastGVK = gvk
	s.lastNS = ns
	s.lastName = name
	return s.getResult, s.getErr
}

func (s *stubResourceReader) ListResources(_ context.Context, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind, ns string, _ metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	s.lastGVR = gvr
	s.lastGVK = gvk
	s.lastNS = ns
	return s.listResult, s.listErr
}

var _ tools.ResourceReader = (*stubResourceReader)(nil)

var _ = Describe("Fleet routing for AF kubectl tools [BR-FLEET-054, AC-3]", func() {

	Describe("ResourceReaderFactory routing", func() {
		It("UT-AF-054-001: routes to fleet reader for non-empty clusterID", func() {
			localReader := &stubResourceReader{}
			fleetReader := &stubResourceReader{
				getResult: newUnstructuredService("remote-ns", "remote-svc", "10.1.0.1"),
			}

			factory := tools.ResourceReaderFactory(func(_ context.Context, clusterID string) (tools.ResourceReader, error) {
				if clusterID == "" {
					return localReader, nil
				}
				return fleetReader, nil
			})

			reader, err := factory(context.Background(), "cluster-east")
			Expect(err).NotTo(HaveOccurred())

			gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
			gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
			obj, err := reader.GetResource(context.Background(), gvr, gvk, "remote-ns", "remote-svc")
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetName()).To(Equal("remote-svc"))

			Expect(fleetReader.lastNS).To(Equal("remote-ns"))
			Expect(fleetReader.lastName).To(Equal("remote-svc"))
			Expect(localReader.lastName).To(BeEmpty(), "local reader should not have been called")
		})

		It("UT-AF-054-002: routes to local reader for empty clusterID", func() {
			localReader := &stubResourceReader{
				getResult: newUnstructuredService("default", "local-svc", "10.0.0.1"),
			}
			fleetReader := &stubResourceReader{}

			factory := tools.ResourceReaderFactory(func(_ context.Context, clusterID string) (tools.ResourceReader, error) {
				if clusterID == "" {
					return localReader, nil
				}
				return fleetReader, nil
			})

			reader, err := factory(context.Background(), "")
			Expect(err).NotTo(HaveOccurred())

			gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
			gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
			obj, err := reader.GetResource(context.Background(), gvr, gvk, "default", "local-svc")
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetName()).To(Equal("local-svc"))

			Expect(localReader.lastName).To(Equal("local-svc"))
			Expect(fleetReader.lastName).To(BeEmpty(), "fleet reader should not have been called")
		})

		It("UT-AF-054-003: returns error when fleet reader factory fails for unknown cluster", func() {
			factory := tools.ResourceReaderFactory(func(_ context.Context, clusterID string) (tools.ResourceReader, error) {
				if clusterID == "" {
					return &stubResourceReader{}, nil
				}
				return nil, fmt.Errorf("unknown cluster %q", clusterID)
			})

			_, err := factory(context.Background(), "nonexistent-cluster")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown cluster"))
		})
	})

	Describe("DynamicResourceReader adapter", func() {
		It("UT-AF-054-004: DynamicResourceReader delegates Get to dynamic.Interface", func() {
			scheme := runtime.NewScheme()
			dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
				newUnstructuredService("prod", "web-svc", "10.0.0.1"),
			)
			reader := &tools.DynamicResourceReader{Client: dynClient}

			gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
			gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
			obj, err := reader.GetResource(context.Background(), gvr, gvk, "prod", "web-svc")
			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetName()).To(Equal("web-svc"))
			Expect(obj.GetNamespace()).To(Equal("prod"))
		})

		It("UT-AF-054-005: DynamicResourceReader delegates List to dynamic.Interface", func() {
			scheme := runtime.NewScheme()
			dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
				newUnstructuredService("prod", "svc-a", "10.0.0.1"),
				newUnstructuredService("prod", "svc-b", "10.0.0.2"),
			)
			reader := &tools.DynamicResourceReader{Client: dynClient}

			gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
			gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
			list, err := reader.ListResources(context.Background(), gvr, gvk, "prod", metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(HaveLen(2))
		})
	})

	Describe("HandleKubectlGet with ResourceReader", func() {
		It("UT-AF-054-006: HandleKubectlGet works with stubResourceReader (fleet path)", func() {
			reader := &stubResourceReader{
				getResult: newUnstructuredService("remote-ns", "remote-svc", "10.1.0.1"),
			}

			result, err := tools.HandleKubectlGet(context.Background(), reader, nil, tools.KubectlGetArgs{
				Kind:      "Service",
				Name:      "remote-svc",
				Namespace: "remote-ns",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("remote-svc"))
			Expect(result.Kind).To(Equal("Service"))
			Expect(result.Object).To(HaveKey("spec"))
		})
	})

	Describe("HandleKubectlList with ResourceReader", func() {
		It("UT-AF-054-007: HandleKubectlList works with stubResourceReader (fleet path)", func() {
			reader := &stubResourceReader{
				listResult: &unstructured.UnstructuredList{
					Items: []unstructured.Unstructured{
						*newUnstructuredService("remote-ns", "svc-a", "10.1.0.1"),
						*newUnstructuredService("remote-ns", "svc-b", "10.1.0.2"),
					},
				},
			}

			result, err := tools.HandleKubectlList(context.Background(), reader, nil, tools.KubectlListArgs{
				Kind:      "Service",
				Namespace: "remote-ns",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(2))
		})
	})
})
