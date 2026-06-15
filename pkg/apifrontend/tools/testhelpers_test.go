package tools_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func objMeta(namespace, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
}

// Legacy helpers — will be removed as each test file migrates to typed client (#1428).

func newFakeRR(namespace, name, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationRequest",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"targetResource": map[string]interface{}{
					"kind": "Deployment",
					"name": "api-server",
				},
			},
			"status": map[string]interface{}{
				"overallPhase": phase,
			},
		},
	}
}

func newUnstructuredRR(ns, name, phase, targetKind, targetName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationRequest",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"signalFingerprint": func() string {
					h := [32]byte{}
					copy(h[:], ns+"/"+targetKind+"/"+targetName)
					return fmt.Sprintf("%x", h)
				}(),
				"targetResource": map[string]interface{}{
					"kind": targetKind,
					"name": targetName,
				},
			},
			"status": map[string]interface{}{
				"overallPhase": phase,
			},
		},
	}
}

func newDynamicFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequestList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationApprovalRequestList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "SignalProcessingList"},
		&unstructured.UnstructuredList{},
	)
	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
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
