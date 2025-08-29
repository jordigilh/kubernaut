package testenv

import (
	"context"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/k8s"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// setupFakeK8sEnvironment creates a fake Kubernetes environment for testing
func setupFakeK8sEnvironment() (*TestEnvironment, error) {
	// Create scheme with necessary objects
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)

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

// CreateDefaultNamespace creates the default namespace in the environment
func (te *TestEnvironment) CreateDefaultNamespace() error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_, err := te.Client.CoreV1().Namespaces().Create(te.Context, ns, metav1.CreateOptions{})
	return err
}

// CreateK8sClient creates a k8s.Client using the test environment
func (te *TestEnvironment) CreateK8sClient(logger *logrus.Logger) k8s.Client {
	return k8s.NewUnifiedClient(te.Client, config.KubernetesConfig{
		Namespace: "default",
	}, logger)
}

