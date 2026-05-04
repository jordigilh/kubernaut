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

package audit

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// K8sCallAuditorImpl implements transport.K8sCallAuditor by emitting
// aiagent.interactive.k8s_call audit events to the audit store.
type K8sCallAuditorImpl struct {
	store  AuditStore
	logger logr.Logger
}

// NewK8sCallAuditor creates a K8sCallAuditor backed by the given audit store.
func NewK8sCallAuditor(store AuditStore, logger logr.Logger) *K8sCallAuditorImpl {
	return &K8sCallAuditorImpl{store: store, logger: logger}
}

func (a *K8sCallAuditorImpl) AuditK8sCall(ctx context.Context, info transport.K8sCallInfo) {
	event := NewEvent(EventTypeInteractiveK8sCall, "", WithSessionID(info.SessionID), WithActingUser(info.User))
	event.EventAction = ActionInteractiveK8sCall
	event.EventOutcome = OutcomeSuccess
	if info.StatusCode >= 400 {
		event.EventOutcome = OutcomeFailure
	}
	event.Data["resource"] = info.Resource
	event.Data["verb"] = info.Verb
	event.Data["namespace"] = info.Namespace
	event.Data["resource_name"] = info.ResourceName
	event.Data["http_status_code"] = info.StatusCode

	StoreBestEffort(context.Background(), a.store, event, a.logger)
}

var _ transport.K8sCallAuditor = (*K8sCallAuditorImpl)(nil)
