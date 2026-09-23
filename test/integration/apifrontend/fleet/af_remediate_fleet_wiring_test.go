package fleet_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

type runnableFleetTool interface {
	tool.Tool
	Run(ctx agent.Context, args any) (map[string]any, error)
}

var _ = Describe("Fleet-mode AF remediation RR/severity wiring [BR-FLEET-054, BR-INTEGRATION-065]", func() {
	It("IT-AF-2394-011: registered remediate tool persists severity from the requested hub cluster", func() {
		ctx := context.Background()
		controllerNS, workloadNS, targetName := newFleetFixtureNames()
		createFleetNamespace(ctx, controllerNS)
		DeferCleanup(func() { deleteFleetNamespace(ctx, controllerNS) })

		clusterRegistry := newFleetTestRegistry()
		remoteScope := &fleetTestRemoteScopeChecker{clusterID: "hub"}
		auditor := &fleetTestAuditRecorder{}
		promClient := &fleetTestPromClient{alerts: []prom.Alert{
			fleetAlert("HubNodePressure", "hub", workloadNS, targetName, severity.SeverityWarning),
			fleetAlert("SpokeNodePressure", "remote-cluster", workloadNS, targetName, severity.SeverityCritical),
		}}

		remediateTool := newFleetRemediateTool(clusterRegistry, remoteScope, auditor, promClient, controllerNS)
		result, err := remediateTool.Run(newFleetToolContext("it-af-2394-011"), map[string]any{
			"namespace":   workloadNS,
			"kind":        "Deployment",
			"name":        targetName,
			"description": "fleet cluster attribution integration test",
			"api_version": "apps/v1",
			"cluster_id":  "hub",
		})
		Expect(err).NotTo(HaveOccurred())
		rrID, ok := result["rr_id"].(string)
		Expect(ok).To(BeTrue())
		Expect(rrID).NotTo(BeEmpty())
		Expect(result["severity"]).To(Equal(severity.SeverityWarning),
			"the higher-severity spoke alert must not influence a hub-targeted RR")
		Expect(result["cluster_id"]).To(Equal("hub"))

		var created remediationv1.RemediationRequest
		Expect(fleetTypedClient.Get(ctx, types.NamespacedName{Namespace: controllerNS, Name: rrID}, &created)).To(Succeed())
		Expect(created.Spec.ClusterID).To(Equal("hub"))
		Expect(created.Spec.Severity).To(Equal(severity.SeverityWarning))
		Expect(created.Spec.SignalName).To(Equal("HubNodePressure"))

		Expect(remoteScope.calls).To(HaveLen(1))
		Expect(remoteScope.calls[0].ClusterID).To(Equal("hub"),
			"fleet-mode hub targets must use the Gateway-routed remote scope checker")
		Expect(rrCreatedAudit(auditor.events, rrID)).To(BeTrue(),
			"AU-3: the RR-created audit event must retain the fleet cluster attribution")
	})

	It("IT-AF-2394-012: fleet remediate fails closed when only another cluster has matching alert evidence", func() {
		ctx := context.Background()
		controllerNS, workloadNS, targetName := newFleetFixtureNames()
		createFleetNamespace(ctx, controllerNS)
		DeferCleanup(func() { deleteFleetNamespace(ctx, controllerNS) })

		clusterRegistry := newFleetTestRegistry()
		remoteScope := &fleetTestRemoteScopeChecker{clusterID: "hub"}
		promClient := &fleetTestPromClient{alerts: []prom.Alert{
			fleetAlert("SpokeNodePressure", "remote-cluster", workloadNS, targetName, severity.SeverityCritical),
		}}

		remediateTool := newFleetRemediateTool(clusterRegistry, remoteScope, nil, promClient, controllerNS)
		result, err := remediateTool.Run(newFleetToolContext("it-af-2394-012"), map[string]any{
			"namespace":   workloadNS,
			"kind":        "Deployment",
			"name":        targetName,
			"description": "fleet cluster attribution fail-closed integration test",
			"api_version": "apps/v1",
			"cluster_id":  "hub",
		})
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, severity.ErrSeverityUndetermined)).To(BeTrue())
		Expect(result).To(BeNil())
		Expect(remoteScope.calls).To(HaveLen(1),
			"scope authorization must succeed first so this assertion isolates severity attribution")

		var requests remediationv1.RemediationRequestList
		Expect(fleetTypedClient.List(ctx, &requests, crclient.InNamespace(controllerNS))).To(Succeed())
		Expect(requests.Items).To(BeEmpty(), "no RR may be created from another cluster's alert")
	})
})

func newFleetFixtureNames() (controllerNamespace, workloadNamespace, targetName string) {
	id := uuid.NewString()[:8]
	return "fleet-ctrl-" + id, "fleet-workload-" + id, "web-" + id
}

func createFleetNamespace(ctx context.Context, name string) {
	Expect(fleetTypedClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
}

func deleteFleetNamespace(ctx context.Context, name string) {
	err := fleetTypedClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	if !apierrors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

func newFleetTestRegistry() *fleetTestClusterRegistry {
	return &fleetTestClusterRegistry{clusters: []registry.ClusterInfo{
		{ID: "hub", MCPEndpoint: "https://hub-gateway.test", ToolPrefix: "hub__"},
		{ID: "remote-cluster", MCPEndpoint: "https://remote-gateway.test", ToolPrefix: "remote-cluster__"},
	}}
}

func fleetAlert(alertName, clusterID, namespace, targetName, severityValue string) prom.Alert {
	return prom.Alert{
		State: "firing",
		Labels: map[string]string{
			"alertname": alertName,
			"cluster":   clusterID,
			"kind":      "Deployment",
			"name":      targetName,
			"namespace": namespace,
			"severity":  severityValue,
		},
	}
}

func newFleetRemediateTool(clusterRegistry *fleetTestClusterRegistry, remoteScope *fleetTestRemoteScopeChecker, auditor audit.Emitter, promClient prom.Client, controllerNamespace string) runnableFleetTool {
	localScope := scope.NewManager(fleetTypedClient)
	federatedScope := fleet.NewFederatedScopeChecker(
		localScope,
		remoteScope,
		logr.Discard(),
		fleet.WithClusterLookup(registry.NewClusterLookupAdapter(clusterRegistry)),
	)
	triager := severity.NewTriager(
		promClient,
		severity.NewNoopLLMTriager(logr.Discard()),
		severity.DefaultConfig(),
		logr.Discard(),
	)

	cfg := agentpkg.DefaultTestConfig()
	cfg.K8sClient = fleetDynamicClient
	cfg.TypedClient = fleetTypedClient
	cfg.Namespace = controllerNamespace
	cfg.Triager = triager
	cfg.Auditor = auditor
	cfg.ScopeChecker = federatedScope
	cfg.ClusterRegistry = clusterRegistry
	cfg.FleetReaderFactory = func(context.Context, string) (tools.ResourceReader, error) {
		return nil, fmt.Errorf("fleet resource reader is not used by the remediate tool")
	}
	_, registeredTools, err := agentpkg.NewRootAgent(cfg)
	Expect(err).NotTo(HaveOccurred())

	for _, registeredTool := range registeredTools {
		if registeredTool.Name() != "kubernaut_remediate" {
			continue
		}
		runnable, ok := registeredTool.(runnableFleetTool)
		Expect(ok).To(BeTrue(), "registered kubernaut_remediate must expose the ADK Run contract")
		return runnable
	}
	Fail("NewRootAgent did not register kubernaut_remediate")
	return nil
}

func newFleetToolContext(callID string) agent.Context {
	ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
		Username: "fleet-it-user",
		Groups:   []string{"sre"},
	})
	return fleetToolContext{Context: ctx, callID: callID}
}

func rrCreatedAudit(events []*audit.Event, rrID string) bool {
	for _, event := range events {
		if event.Type == audit.EventRRCreated && event.CorrelationID == rrID && event.ClusterID == "hub" && event.Detail["cluster_id"] == "hub" {
			return true
		}
	}
	return false
}

type fleetTestClusterRegistry struct {
	clusters []registry.ClusterInfo
}

func (*fleetTestClusterRegistry) Start(context.Context) error { return nil }
func (*fleetTestClusterRegistry) Stop()                       {}
func (*fleetTestClusterRegistry) Ready() bool                 { return true }
func (*fleetTestClusterRegistry) WatchClusters() <-chan registry.ClusterEvent {
	return nil
}

func (r *fleetTestClusterRegistry) List() []registry.ClusterInfo {
	return append([]registry.ClusterInfo(nil), r.clusters...)
}

func (r *fleetTestClusterRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	for _, cluster := range r.clusters {
		if cluster.ID == clusterID {
			return cluster, true
		}
	}
	return registry.ClusterInfo{}, false
}

type fleetTestRemoteScopeChecker struct {
	clusterID string
	calls     []scope.ResourceIdentity
}

func (c *fleetTestRemoteScopeChecker) IsManagedResource(_ context.Context, resource scope.ResourceIdentity) (bool, error) {
	c.calls = append(c.calls, resource)
	return resource.ClusterID == c.clusterID, nil
}

type fleetTestAuditRecorder struct {
	events []*audit.Event
}

func (r *fleetTestAuditRecorder) Emit(_ context.Context, event *audit.Event) {
	copiedEvent := *event
	r.events = append(r.events, &copiedEvent)
}

type fleetTestPromClient struct {
	alerts []prom.Alert
}

func (c *fleetTestPromClient) GetAlerts(context.Context) ([]prom.Alert, error) {
	return c.alerts, nil
}

func (*fleetTestPromClient) GetRules(context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}

func (*fleetTestPromClient) InstantQuery(context.Context, string) (*prom.QueryResult, error) {
	return &prom.QueryResult{}, nil
}

type embeddedFleetAgentContext interface {
	agent.Context
}

type fleetToolContext struct {
	embeddedFleetAgentContext
	context.Context
	callID string
}

func (c fleetToolContext) Deadline() (time.Time, bool) { return c.Context.Deadline() }
func (c fleetToolContext) Done() <-chan struct{}       { return c.Context.Done() }
func (c fleetToolContext) Err() error                  { return c.Context.Err() }
func (c fleetToolContext) Value(key any) any           { return c.Context.Value(key) }
func (c fleetToolContext) FunctionCallID() string      { return c.callID }
func (fleetToolContext) UserID() string                { return "fleet-it-user" }
func (fleetToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation {
	return nil
}
