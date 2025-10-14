/*
Copyright 2025 Kubernaut.

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

package notification

import (
	"context"
	"fmt"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client provides operations for NotificationRequest CRDs
// This interface abstracts Kubernetes client operations for notification resources,
// enabling clean integration with RemediationOrchestrator and other controllers.
//
// Usage in RemediationOrchestrator:
//
//	notifClient := notification.NewClient(k8sClient)
//	err := notifClient.Create(ctx, &notificationv1alpha1.NotificationRequest{
//	    ObjectMeta: metav1.ObjectMeta{Name: "alert-notification", Namespace: "default"},
//	    Spec: notificationv1alpha1.NotificationRequestSpec{...},
//	})
type Client interface {
	// Create creates a new notification request
	// Returns error if creation fails or if notification already exists
	Create(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error

	// Get retrieves a notification request by name and namespace
	// Returns the notification or error if not found
	Get(ctx context.Context, name, namespace string) (*notificationv1alpha1.NotificationRequest, error)

	// List lists all notification requests in a namespace
	// Pass empty string for namespace to list across all namespaces
	List(ctx context.Context, namespace string, opts ...client.ListOption) (*notificationv1alpha1.NotificationRequestList, error)

	// Update updates an existing notification request
	// Note: Updates to spec may be rejected by the controller based on current phase
	Update(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error

	// Delete deletes a notification request
	// Uses foreground deletion policy to ensure cleanup
	Delete(ctx context.Context, name, namespace string) error

	// UpdateStatus updates the status subresource
	// This is used by the controller to update delivery status, phase, etc.
	UpdateStatus(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error

	// Watch returns a channel that receives notification events
	// Useful for monitoring notification lifecycle in integration tests
	// Note: Close the returned channel when done watching
	Watch(ctx context.Context, namespace string) (<-chan Event, error)
}

// Event represents a notification event for watching
type Event struct {
	Type         EventType
	Notification *notificationv1alpha1.NotificationRequest
	Error        error
}

// EventType represents the type of notification event
type EventType string

const (
	// EventAdded indicates a notification was created
	EventAdded EventType = "ADDED"
	// EventModified indicates a notification was updated
	EventModified EventType = "MODIFIED"
	// EventDeleted indicates a notification was deleted
	EventDeleted EventType = "DELETED"
	// EventError indicates an error occurred during watch
	EventError EventType = "ERROR"
)

// notificationClient implements the Client interface
type notificationClient struct {
	client client.Client
}

// NewClient creates a new notification client
// The k8sClient should be a controller-runtime client with NotificationRequest scheme registered
func NewClient(k8sClient client.Client) Client {
	return &notificationClient{
		client: k8sClient,
	}
}

// Create creates a new notification request
func (c *notificationClient) Create(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error {
	if notif == nil {
		return fmt.Errorf("notification request cannot be nil")
	}

	if err := c.client.Create(ctx, notif); err != nil {
		return fmt.Errorf("failed to create notification request %s/%s: %w",
			notif.Namespace, notif.Name, err)
	}

	return nil
}

// Get retrieves a notification request by name and namespace
func (c *notificationClient) Get(ctx context.Context, name, namespace string) (*notificationv1alpha1.NotificationRequest, error) {
	if name == "" {
		return nil, fmt.Errorf("notification name cannot be empty")
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace cannot be empty")
	}

	notif := &notificationv1alpha1.NotificationRequest{}
	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	if err := c.client.Get(ctx, key, notif); err != nil {
		return nil, fmt.Errorf("failed to get notification request %s/%s: %w",
			namespace, name, err)
	}

	return notif, nil
}

// List lists all notification requests in a namespace
func (c *notificationClient) List(ctx context.Context, namespace string, opts ...client.ListOption) (*notificationv1alpha1.NotificationRequestList, error) {
	list := &notificationv1alpha1.NotificationRequestList{}

	// If namespace is specified, add it to list options
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}

	if err := c.client.List(ctx, list, opts...); err != nil {
		if namespace != "" {
			return nil, fmt.Errorf("failed to list notifications in namespace %s: %w", namespace, err)
		}
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}

	return list, nil
}

// Update updates an existing notification request
func (c *notificationClient) Update(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error {
	if notif == nil {
		return fmt.Errorf("notification request cannot be nil")
	}

	if err := c.client.Update(ctx, notif); err != nil {
		return fmt.Errorf("failed to update notification request %s/%s: %w",
			notif.Namespace, notif.Name, err)
	}

	return nil
}

// Delete deletes a notification request
func (c *notificationClient) Delete(ctx context.Context, name, namespace string) error {
	if name == "" {
		return fmt.Errorf("notification name cannot be empty")
	}
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	notif := &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	// Use foreground deletion policy to ensure cleanup
	deleteOptions := client.DeleteOptions{}
	propagationPolicy := metav1.DeletePropagationForeground
	deleteOptions.PropagationPolicy = &propagationPolicy

	if err := c.client.Delete(ctx, notif, &deleteOptions); err != nil {
		return fmt.Errorf("failed to delete notification request %s/%s: %w",
			namespace, name, err)
	}

	return nil
}

// UpdateStatus updates the status subresource
func (c *notificationClient) UpdateStatus(ctx context.Context, notif *notificationv1alpha1.NotificationRequest) error {
	if notif == nil {
		return fmt.Errorf("notification request cannot be nil")
	}

	if err := c.client.Status().Update(ctx, notif); err != nil {
		return fmt.Errorf("failed to update notification request status %s/%s: %w",
			notif.Namespace, notif.Name, err)
	}

	return nil
}

// Watch returns a channel that receives notification events
// Note: This is a simplified implementation for integration tests
// Production use should implement proper watch with reconnection logic
func (c *notificationClient) Watch(ctx context.Context, namespace string) (<-chan Event, error) {
	// Note: controller-runtime client doesn't directly support watch
	// For integration tests, use periodic List() calls instead
	// For production, use client-go's watch directly on the RESTClient
	return nil, fmt.Errorf("watch is not implemented - use periodic List() calls for integration tests")
}

// Helper functions for common notification creation patterns

// CreateEscalationNotification creates a notification for alert escalation
// This is a convenience method for RemediationOrchestrator to create escalation notifications
func CreateEscalationNotification(
	name, namespace string,
	subject, body string,
	recipients []notificationv1alpha1.Recipient,
	channels []notificationv1alpha1.Channel,
	metadata map[string]string,
) *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:       notificationv1alpha1.NotificationTypeEscalation,
			Priority:   notificationv1alpha1.NotificationPriorityCritical,
			Subject:    subject,
			Body:       body,
			Recipients: recipients,
			Channels:   channels,
			Metadata:   metadata,
		},
	}
}

// CreateStatusUpdateNotification creates a notification for status updates
// Used by RemediationOrchestrator to notify about remediation progress
func CreateStatusUpdateNotification(
	name, namespace string,
	subject, body string,
	recipients []notificationv1alpha1.Recipient,
	channels []notificationv1alpha1.Channel,
	metadata map[string]string,
) *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:       notificationv1alpha1.NotificationTypeStatusUpdate,
			Priority:   notificationv1alpha1.NotificationPriorityMedium,
			Subject:    subject,
			Body:       body,
			Recipients: recipients,
			Channels:   channels,
			Metadata:   metadata,
		},
	}
}

// CreateSimpleNotification creates a simple notification
// Used for informational messages
func CreateSimpleNotification(
	name, namespace string,
	subject, body string,
	recipients []notificationv1alpha1.Recipient,
	channels []notificationv1alpha1.Channel,
) *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:       notificationv1alpha1.NotificationTypeSimple,
			Priority:   notificationv1alpha1.NotificationPriorityLow,
			Subject:    subject,
			Body:       body,
			Recipients: recipients,
			Channels:   channels,
		},
	}
}
