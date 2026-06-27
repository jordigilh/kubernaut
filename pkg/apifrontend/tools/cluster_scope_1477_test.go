package tools_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// logCaptureSink captures log messages for test assertions (AU-3).
type logCaptureSink struct {
	messages []string
	kvPairs  []map[string]interface{}
}

func (s *logCaptureSink) Init(logr.RuntimeInfo)                    {}
func (s *logCaptureSink) Enabled(int) bool                         { return true }
func (s *logCaptureSink) WithValues(...interface{}) logr.LogSink   { return s }
func (s *logCaptureSink) WithName(string) logr.LogSink             { return s }
func (s *logCaptureSink) Error(error, string, ...interface{})      {}
func (s *logCaptureSink) Info(_ int, msg string, keysAndValues ...interface{}) {
	s.messages = append(s.messages, msg)
	kv := make(map[string]interface{})
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		kv[keysAndValues[i].(string)] = keysAndValues[i+1]
	}
	s.kvPairs = append(s.kvPairs, kv)
}

func newScopeAwareMapper() *meta.DefaultRESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "config.openshift.io", Version: "v1"},
	})
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "config.openshift.io", Version: "v1", Kind: "ClusterOperator"}, meta.RESTScopeRoot)
	return m
}

var clusterScopeGVRs = map[schema.GroupVersionResource]string{
	{Group: "", Version: "v1", Resource: "nodes"}:                               "NodeList",
	{Group: "", Version: "v1", Resource: "namespaces"}:                          "NamespaceList",
	{Group: "apps", Version: "v1", Resource: "deployments"}:                     "DeploymentList",
	{Group: "config.openshift.io", Version: "v1", Resource: "clusteroperators"}: "ClusterOperatorList",
}

func newUnstructuredNode(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{},
			},
		},
	}
}

func newUnstructuredNamespace(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
}

var _ = Describe("Cluster-scoped namespace stripping (#1477)", func() {

	Describe("HandleInvestigateAlert — self-healing strip [SC-5, SI-10]", func() {
		It("UT-AF-1477-001: strips namespace for cluster-scoped resource and proceeds", func() {
			sink := &logCaptureSink{}
			logger := logr.New(sink)
			ctx := logr.NewContext(context.Background(), logger)

			cfg := tools.InvestigateAlertConfig{
				Client:       newTypedFakeClient(),
				ControllerNS: "kubernaut-system",
				Mapper:       newScopeAwareMapper(),
			}
			result, err := tools.HandleInvestigateAlert(ctx, cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubeNodeNotReady",
					APIVersion: "v1",
					Kind:       "Node",
					Name:       "worker-1",
					Namespace:  "openshift-cluster-version",
				}, "admin")
			Expect(err).NotTo(HaveOccurred(), "cluster-scoped resource with namespace should succeed (strip + proceed)")
			Expect(result.RRID).NotTo(BeEmpty())
			Expect(sink.messages).To(ContainElement("stripping namespace for cluster-scoped resource"))
		})

		It("UT-AF-1477-005: namespaced resource without namespace still errors [SI-10]", func() {
			cfg := tools.InvestigateAlertConfig{
				Client:       newTypedFakeClient(),
				ControllerNS: "kubernaut-system",
				Mapper:       newScopeAwareMapper(),
			}
			_, err := tools.HandleInvestigateAlert(context.Background(), cfg,
				&tools.InvestigateAlertArgs{
					AlertName:  "KubePodCrashLooping",
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "web",
				}, "user")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespaced but namespace was not provided"))
		})
	})

	Describe("HandleKubectlGet — scope-aware routing [SC-5]", func() {
		It("UT-AF-1477-002: succeeds for cluster-scoped resource with namespace provided", func() {
			sink := &logCaptureSink{}
			logger := logr.New(sink)
			ctx := logr.NewContext(context.Background(), logger)

			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, clusterScopeGVRs,
				newUnstructuredNode("worker-1"),
			)
			mapper := newScopeAwareMapper()

			result, err := tools.HandleKubectlGet(ctx, &tools.DynamicResourceReader{Client: client}, mapper, tools.KubectlGetArgs{
				Kind:      "Node",
				Name:      "worker-1",
				Namespace: "default",
			})
			Expect(err).NotTo(HaveOccurred(), "cluster-scoped Get with namespace should succeed (strip + proceed)")
			Expect(result.Kind).To(Equal("Node"))
			Expect(result.Name).To(Equal("worker-1"))
			Expect(sink.messages).To(ContainElement("stripping namespace for cluster-scoped resource"))
		})
	})

	Describe("HandleKubectlList — scope-aware routing [SC-5]", func() {
		It("UT-AF-1477-003: succeeds for cluster-scoped resource with namespace provided", func() {
			sink := &logCaptureSink{}
			logger := logr.New(sink)
			ctx := logr.NewContext(context.Background(), logger)

			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, clusterScopeGVRs,
				newUnstructuredNamespace("default"),
				newUnstructuredNamespace("kube-system"),
			)
			mapper := newScopeAwareMapper()

			result, err := tools.HandleKubectlList(ctx, &tools.DynamicResourceReader{Client: client}, mapper, tools.KubectlListArgs{
				Kind:      "Namespace",
				Namespace: "default",
			})
			Expect(err).NotTo(HaveOccurred(), "cluster-scoped List with namespace should succeed (strip + proceed)")
			Expect(result.Count).To(BeNumerically(">=", 1))
			Expect(sink.messages).To(ContainElement("stripping namespace for cluster-scoped resource"))
		})
	})

	Describe("HandleInvestigateMCP — RESTMapper scope detection [SI-10]", func() {
		It("UT-AF-1477-004: sets ClusterScoped correctly via dynamic mapper when namespace provided", func() {
			sink := &logCaptureSink{}
			logger := logr.New(sink)
			mapper := newScopeAwareMapper()
			ctx := logr.NewContext(context.Background(), logger)
			ctx = tools.ContextWithRESTMapper(ctx, mapper)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					ch := make(chan ka.InvestigationEvent, 1)
					return &ka.StartInvestigationResult{
						SessionID: "sess-scope-001",
						Status:    "autonomous_started",
						Events:    ch,
						Closer:    func() { close(ch) },
					}, nil
				},
			}

			cfg := tools.InvestigateAlertConfig{
				Client:       newTypedFakeClient(),
				ControllerNS: "kubernaut-system",
				Mapper:       mapper,
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, &tools.InvestigateConfig{
					MCPClient: mockMCP,
					Client:    cfg.Client,
					Namespace: "kubernaut-system",
				}, tools.InvestigateMCPArgs{
					APIVersion: "v1",
					Kind:       "Node",
					Name:       "worker-1",
					Namespace:  "kube-system",
				}, false, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).NotTo(BeEmpty())
			Expect(sink.messages).To(ContainElement("stripping namespace for cluster-scoped resource"))
		})
	})

	Describe("resolveEffectiveNamespace — AU-3 logging", func() {
		It("UT-AF-1477-006: logs warning with kind, apiVersion, stripped_namespace on strip", func() {
			mapper := newScopeAwareMapper()
			sink := &logCaptureSink{}
			logger := logr.New(sink)

			ns := tools.ResolveEffectiveNamespace(mapper, "Node", "kube-system", logger)
			Expect(ns).To(Equal(""))
			Expect(sink.messages).To(HaveLen(1))
			Expect(sink.messages[0]).To(ContainSubstring("stripping namespace"))
			Expect(sink.kvPairs[0]).To(HaveKeyWithValue("kind", "Node"))
			Expect(sink.kvPairs[0]).To(HaveKeyWithValue("apiVersion", "v1"))
			Expect(sink.kvPairs[0]).To(HaveKeyWithValue("stripped_namespace", "kube-system"))
		})
	})
})
