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

package authwebhook

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateNamespace creates a Kubernetes namespace for testing
func CreateNamespace(ctx context.Context, k8sClient client.Client, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return k8sClient.Create(ctx, ns)
}

// DeleteNamespace deletes a Kubernetes namespace
func DeleteNamespace(ctx context.Context, k8sClient client.Client, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return client.IgnoreNotFound(k8sClient.Delete(ctx, ns))
}

// ValidateEventData validates that event_data contains expected fields
func ValidateEventData(eventData map[string]interface{}, expectedFields map[string]interface{}) error {
	for key, expectedValue := range expectedFields {
		actualValue, ok := eventData[key]
		if !ok {
			return fmt.Errorf("event_data missing required field: %s", key)
		}

		// Skip nil value checks (field exists but may be empty)
		if expectedValue == nil {
			continue
		}

		if actualValue != expectedValue {
			return fmt.Errorf("event_data[%s] = %v, expected %v", key, actualValue, expectedValue)
		}
	}
	return nil
}
