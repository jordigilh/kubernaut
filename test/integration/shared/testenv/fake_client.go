package testenv

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// setupFakeK8sEnvironment creates a fake Kubernetes environment for testing
func setupFakeK8sEnvironment() (*TestEnvironment, error) {
	// Create scheme with necessary objects
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	// Create fake client
	client := fake.NewSimpleClientset()

	ctx, cancel := context.WithCancel(context.Background())

	env := &TestEnvironment{
		Environment: nil, // No envtest environment for fake
		Config:      nil, // No real config for fake
		Client:      client,
		Context:     ctx,
		CancelFunc:  cancel,
	}

	return env, nil
}
