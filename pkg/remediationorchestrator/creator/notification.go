/*
Copyright 2025 Jordi Gil.

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

package creator

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NotificationCreator creates NotificationRequest CRDs for the Remediation Orchestrator.
// Reference: BR-ORCH-001 (approval notification), BR-ORCH-034 (bulk duplicate), BR-ORCH-036 (manual review)
type NotificationCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewNotificationCreator creates a new NotificationCreator.
func NewNotificationCreator(c client.Client, s *runtime.Scheme) *NotificationCreator {
	return &NotificationCreator{
		client: c,
		scheme: s,
	}
}

