package toolset

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

var _ = Describe("Detector Utilities", func() {
	Describe("HasAnyLabel", func() {
		DescribeTable("should detect if service has any of the specified labels",
			func(serviceLabels map[string]string, checkLabels map[string]string, expected bool) {
				service := &corev1.Service{
					Spec: corev1.ServiceSpec{},
				}
				service.Labels = serviceLabels

				result := discovery.HasAnyLabel(service, checkLabels)
				Expect(result).To(Equal(expected))
			},
			Entry("matches first label",
				map[string]string{"app": "prometheus", "version": "v1"},
				map[string]string{"app": "prometheus", "component": "monitoring"},
				true,
			),
			Entry("matches second label",
				map[string]string{"app": "grafana", "version": "v1"},
				map[string]string{"component": "monitoring", "app": "grafana"},
				true,
			),
			Entry("matches with different value (should not match)",
				map[string]string{"app": "prometheus"},
				map[string]string{"app": "grafana"},
				false,
			),
			Entry("no labels match",
				map[string]string{"app": "nginx"},
				map[string]string{"component": "database", "tier": "backend"},
				false,
			),
			Entry("empty service labels",
				map[string]string{},
				map[string]string{"app": "prometheus"},
				false,
			),
			Entry("empty check labels",
				map[string]string{"app": "prometheus"},
				map[string]string{},
				false,
			),
		)
	})

	Describe("BuildHTTPSEndpoint", func() {
		DescribeTable("should build HTTPS endpoint URLs",
			func(serviceName, namespace string, port int32, expected string) {
				result := discovery.BuildHTTPSEndpoint(serviceName, namespace, port)
				Expect(result).To(Equal(expected))
			},
			Entry("standard service",
				"prometheus", "monitoring", int32(9090),
				"https://prometheus.monitoring.svc.cluster.local:9090",
			),
			Entry("default namespace",
				"grafana", "default", int32(3000),
				"https://grafana.default.svc.cluster.local:3000",
			),
			Entry("custom namespace with port 443",
				"api-server", "production", int32(443),
				"https://api-server.production.svc.cluster.local:443",
			),
		)
	})

	Describe("GetPortNumber", func() {
		DescribeTable("should extract port number using priority-based search",
			func(ports []corev1.ServicePort, portNames []string, targetPort int32, fallbackPort int32, expectedPort int32) {
				service := &corev1.Service{
					Spec: corev1.ServiceSpec{
						Ports: ports,
					},
				}

				portNum := discovery.GetPortNumber(service, portNames, targetPort, fallbackPort)
				Expect(portNum).To(Equal(expectedPort))
			},
			Entry("finds port by name",
				[]corev1.ServicePort{
					{Name: "http", Port: 8080},
					{Name: "metrics", Port: 9090},
				},
				[]string{"metrics"},
				int32(0),
				int32(8080),
				int32(9090), // Should find "metrics" port
			),
			Entry("finds port by number",
				[]corev1.ServicePort{
					{Name: "http", Port: 8080},
					{Name: "https", Port: 8443},
				},
				[]string{},
				int32(8443),
				int32(80),
				int32(8443), // Should find port 8443
			),
			Entry("uses first port when no match",
				[]corev1.ServicePort{
					{Name: "http", Port: 8080},
					{Name: "https", Port: 8443},
				},
				[]string{"nonexistent"},
				int32(9999),
				int32(80),
				int32(8080), // Should use first port
			),
			Entry("uses fallback when no ports",
				[]corev1.ServicePort{},
				[]string{"http"},
				int32(0),
				int32(8080),
				int32(8080), // Should use fallback
			),
		)
	})

	Describe("FindPortByNumber", func() {
		It("should handle edge cases", func() {
			service := &corev1.Service{
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						},
						{
							Name:       "https",
							Port:       8443,
							TargetPort: intstr.FromInt(8443),
						},
					},
				},
			}

			// Test finding non-existent port
			port := discovery.FindPortByNumber(service, 9090)
			Expect(port).To(BeNil())

			// Test finding existing port
			port = discovery.FindPortByNumber(service, 8443)
			Expect(port).ToNot(BeNil())
			Expect(port.Name).To(Equal("https"))
		})
	})
})
