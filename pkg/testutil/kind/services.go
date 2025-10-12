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

package kind

import (
	"fmt"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceConfig defines a test service to deploy to Kind cluster.
type ServiceConfig struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Ports     []corev1.ServicePort
	Type      corev1.ServiceType
}

// DeployService creates a Kubernetes service in the Kind cluster.
// Returns the created service or fails the test if deployment fails.
//
// Example:
//
//	svc, err := suite.DeployService(kind.ServiceConfig{
//	    Name:      "my-service",
//	    Namespace: "test-ns",
//	    Labels:    map[string]string{"app": "my-app"},
//	    Ports:     []corev1.ServicePort{{Name: "http", Port: 8080}},
//	})
func (s *IntegrationSuite) DeployService(config ServiceConfig) (*corev1.Service, error) {
	if config.Type == "" {
		config.Type = corev1.ServiceTypeClusterIP
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
			Labels:    config.Labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: config.Labels,
			Ports:    config.Ports,
			Type:     config.Type,
		},
	}

	createdSvc, err := s.Client.CoreV1().Services(config.Namespace).Create(
		s.Context, service, metav1.CreateOptions{})

	if err != nil {
		return nil, err
	}

	// Register cleanup
	s.RegisterCleanup(func() {
		_ = s.Client.CoreV1().Services(config.Namespace).Delete(
			s.Context, config.Name, metav1.DeleteOptions{})
	})

	return createdSvc, nil
}

// DeployPrometheusService deploys a standard Prometheus test service to Kind cluster.
// This creates a service that mimics Prometheus for testing service discovery.
//
// The service will have:
//   - Label: app=prometheus
//   - Port: web (9090)
//   - Type: ClusterIP
//
// Example:
//
//	svc, err := suite.DeployPrometheusService("monitoring")
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) DeployPrometheusService(namespace string) (*corev1.Service, error) {
	return s.DeployService(ServiceConfig{
		Name:      "prometheus",
		Namespace: namespace,
		Labels: map[string]string{
			"app":                          "prometheus",
			"app.kubernetes.io/name":       "prometheus",
			"app.kubernetes.io/component":  "monitoring",
			"app.kubernetes.io/managed-by": "kubernaut-test",
		},
		Ports: []corev1.ServicePort{
			{
				Name:       "web",
				Port:       9090,
				TargetPort: intstr.FromInt(9090),
				Protocol:   corev1.ProtocolTCP,
			},
		},
	})
}

// DeployGrafanaService deploys a standard Grafana test service to Kind cluster.
// This creates a service that mimics Grafana for testing service discovery.
//
// The service will have:
//   - Label: app=grafana
//   - Port: service (3000)
//   - Type: ClusterIP
//
// Example:
//
//	svc, err := suite.DeployGrafanaService("monitoring")
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) DeployGrafanaService(namespace string) (*corev1.Service, error) {
	return s.DeployService(ServiceConfig{
		Name:      "grafana",
		Namespace: namespace,
		Labels: map[string]string{
			"app":                          "grafana",
			"app.kubernetes.io/name":       "grafana",
			"app.kubernetes.io/component":  "monitoring",
			"app.kubernetes.io/managed-by": "kubernaut-test",
		},
		Ports: []corev1.ServicePort{
			{
				Name:       "service",
				Port:       3000,
				TargetPort: intstr.FromInt(3000),
				Protocol:   corev1.ProtocolTCP,
			},
		},
	})
}

// DeployJaegerService deploys a standard Jaeger test service to Kind cluster.
// This creates a service that mimics Jaeger for testing service discovery.
//
// The service will have:
//   - Label: app=jaeger
//   - Port: query (16686)
//   - Type: ClusterIP
//
// Example:
//
//	svc, err := suite.DeployJaegerService("tracing")
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) DeployJaegerService(namespace string) (*corev1.Service, error) {
	return s.DeployService(ServiceConfig{
		Name:      "jaeger",
		Namespace: namespace,
		Labels: map[string]string{
			"app":                          "jaeger",
			"app.kubernetes.io/name":       "jaeger",
			"app.kubernetes.io/component":  "tracing",
			"app.kubernetes.io/managed-by": "kubernaut-test",
		},
		Ports: []corev1.ServicePort{
			{
				Name:       "query",
				Port:       16686,
				TargetPort: intstr.FromInt(16686),
				Protocol:   corev1.ProtocolTCP,
			},
		},
	})
}

// DeployElasticsearchService deploys a standard Elasticsearch test service to Kind cluster.
// This creates a service that mimics Elasticsearch for testing service discovery.
//
// The service will have:
//   - Label: app=elasticsearch
//   - Port: 9200
//   - Type: ClusterIP
//
// Example:
//
//	svc, err := suite.DeployElasticsearchService("logging")
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) DeployElasticsearchService(namespace string) (*corev1.Service, error) {
	return s.DeployService(ServiceConfig{
		Name:      "elasticsearch",
		Namespace: namespace,
		Labels: map[string]string{
			"app":                          "elasticsearch",
			"app.kubernetes.io/name":       "elasticsearch",
			"app.kubernetes.io/component":  "logging",
			"app.kubernetes.io/managed-by": "kubernaut-test",
		},
		Ports: []corev1.ServicePort{
			{
				Name:       "http",
				Port:       9200,
				TargetPort: intstr.FromInt(9200),
				Protocol:   corev1.ProtocolTCP,
			},
		},
	})
}

// GetServiceEndpoint returns the Kubernetes DNS endpoint for a service.
// Format: http://service-name.namespace.svc.cluster.local:port
//
// Example:
//
//	endpoint := suite.GetServiceEndpoint("prometheus", "monitoring", 9090)
//	// Returns: "http://prometheus.monitoring.svc.cluster.local:9090"
func (s *IntegrationSuite) GetServiceEndpoint(serviceName, namespace string, port int32) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serviceName, namespace, port)
}

// DeleteService deletes a service from the Kind cluster.
// If the service doesn't exist, it's treated as success (idempotent).
func (s *IntegrationSuite) DeleteService(namespace, name string) error {
	err := s.Client.CoreV1().Services(namespace).Delete(
		s.Context, name, metav1.DeleteOptions{})

	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

// GetService retrieves a service from the Kind cluster.
func (s *IntegrationSuite) GetService(namespace, name string) (*corev1.Service, error) {
	return s.Client.CoreV1().Services(namespace).Get(
		s.Context, name, metav1.GetOptions{})
}

// ServiceExists checks if a service exists in the Kind cluster.
func (s *IntegrationSuite) ServiceExists(namespace, name string) bool {
	_, err := s.GetService(namespace, name)
	return err == nil
}

// WaitForServiceReady waits for a service to be created in the Kind cluster.
// Uses Gomega's Eventually() for asynchronous assertions.
//
// Example:
//
//	suite.DeployPrometheusService("monitoring")
//	suite.WaitForServiceReady("monitoring", "prometheus")
func (s *IntegrationSuite) WaitForServiceReady(namespace, name string) *corev1.Service {
	var svc *corev1.Service
	Eventually(func() error {
		var err error
		svc, err = s.GetService(namespace, name)
		return err
	}).Should(Succeed(), fmt.Sprintf("Service %s/%s should be ready", namespace, name))

	return svc
}
