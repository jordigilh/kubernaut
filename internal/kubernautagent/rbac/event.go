/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rbac

import (
	"context"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

const (
	ReasonInteractiveSoftDisabled = "InteractiveSoftDisabled"
	ReasonInteractiveEnabled      = "InteractiveEnabled"
)

// EventEmitter wraps a Kubernetes event recorder for KA startup events.
// Events are emitted against the KA pod so they appear in
// `kubectl describe pod` and `kubectl get events`.
type EventEmitter struct {
	recorder  record.EventRecorder
	podObj    *corev1.Pod
	broadcast record.EventBroadcaster
}

// NewEventEmitter creates a recorder backed by the K8s Events API.
// podName and namespace are expected from the downward API env vars
// POD_NAME and POD_NAMESPACE. Returns nil if either is empty (e.g.
// running outside a cluster).
//
// The emitter looks up the Pod's UID so that K8s can properly associate
// and deduplicate events. If the lookup fails (e.g. RBAC), it falls
// back to emitting without UID.
func NewEventEmitter(clientset kubernetes.Interface, podName, namespace string) *EventEmitter {
	if podName == "" || namespace == "" {
		return nil
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: clientset.CoreV1().Events(namespace),
	})
	rec := broadcaster.NewRecorder(scheme, corev1.EventSource{
		Component: "kubernaut-agent",
	})

	podObj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{}); err == nil {
		podObj.UID = pod.UID
	}

	return &EventEmitter{
		recorder:  rec,
		broadcast: broadcaster,
		podObj:    podObj,
	}
}

// EmitInteractiveSoftDisabled emits a Warning event when interactive mode
// is soft-disabled due to missing RBAC (impersonate verb denied).
func (e *EventEmitter) EmitInteractiveSoftDisabled(reason string) {
	if e == nil {
		return
	}
	e.recorder.Event(e.podObj, corev1.EventTypeWarning, ReasonInteractiveSoftDisabled, reason)
}

// EmitInteractiveEnabled emits a Normal event when interactive mode
// starts successfully after passing the SSAR check.
func (e *EventEmitter) EmitInteractiveEnabled() {
	if e == nil {
		return
	}
	e.recorder.Event(e.podObj, corev1.EventTypeNormal, ReasonInteractiveEnabled,
		"Interactive mode enabled: SA has impersonate permission")
}

// Shutdown stops the event broadcaster. Call during graceful shutdown.
func (e *EventEmitter) Shutdown() {
	if e == nil {
		return
	}
	e.broadcast.Shutdown()
}

// DetectPodIdentity reads POD_NAME and POD_NAMESPACE from the downward
// API env vars injected by the operator.
func DetectPodIdentity() (podName, namespace string) {
	return os.Getenv("POD_NAME"), os.Getenv("POD_NAMESPACE")
}
