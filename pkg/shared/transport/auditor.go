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

package transport

import "context"

// K8sCallInfo holds the parsed details of an impersonated K8s API call
// for audit emission (BR-INTERACTIVE-003).
type K8sCallInfo struct {
	User         string
	Groups       []string
	Verb         string
	Resource     string
	Namespace    string
	ResourceName string
	StatusCode   int
	SessionID    string
}

// K8sCallAuditor is the interface for auditing impersonated K8s API calls.
// The implementation lives in internal/kubernautagent/audit and has access
// to session context (session_id, correlation_id) via the provided context.
//
// This interface is intentionally narrow (single method) to avoid coupling
// pkg/shared/transport to the internal audit infrastructure.
type K8sCallAuditor interface {
	AuditK8sCall(ctx context.Context, info K8sCallInfo)
}
