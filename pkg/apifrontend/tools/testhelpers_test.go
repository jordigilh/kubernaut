package tools_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

func objMeta(namespace, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}

func aiaTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = aiav1alpha1.AddToScheme(s)
	return s
}

// newTypedAIAnalysis builds an AIAnalysis fixture for tests that only care
// about the CRD's mere existence for a given RR (e.g. HandleInvestigationMCP's
// AIA-existence poll) -- NOT KA's real session ID, which since DD-AA-KA-001
// lives on AgentSession.Status.SessionID, not AIAnalysis.Status.KASession.ID
// (see crd_tools_session.go's HandleAwaitSession doc comment). sessionID here
// only populates the legacy KASession.ID observability mirror.
func newTypedAIAnalysis(ns, name, rrName, sessionID string) *aiav1alpha1.AIAnalysis {
	aia := &aiav1alpha1.AIAnalysis{
		ObjectMeta: objMeta(ns, name),
		Spec: aiav1alpha1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      rrName,
				Namespace: ns,
			},
			RemediationID: rrName,
		},
	}
	if sessionID != "" {
		aia.Status.KASession = &aiav1alpha1.KASession{
			ID: sessionID,
		}
	}
	return aia
}

func newTypedAIAnalysisClient(objects ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(aiaTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}

func newAgentSessionScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = agentsessionv1.AddToScheme(s)
	return s
}

func watchTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = remediationv1.AddToScheme(s)
	_ = eav1alpha1.AddToScheme(s)
	return s
}

func newForbiddenError(resource string) *errors.StatusError {
	return errors.NewForbidden(
		schema.GroupResource{Group: "kubernaut.ai", Resource: resource},
		"",
		nil,
	)
}

// bridgeQueue captures A2A events written by the EventBridge for test assertions.
type bridgeQueue struct {
	mu     sync.Mutex
	events []a2a.Event
}

func (q *bridgeQueue) Write(_ context.Context, event a2a.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, event)
	return nil
}

func (q *bridgeQueue) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return fmt.Errorf("not supported")
}

func (q *bridgeQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, fmt.Errorf("not supported")
}

func (q *bridgeQueue) Close() error { return nil }

func (q *bridgeQueue) Events() []a2a.Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	cp := make([]a2a.Event, len(q.events))
	copy(cp, q.events)
	return cp
}
