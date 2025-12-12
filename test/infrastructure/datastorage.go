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

package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateDataStorageCluster creates a Kind cluster for Data Storage E2E tests
// This includes:
// - Kind cluster (2 nodes: control-plane + worker)
// - Data Storage Service Docker image (build + load)
func CreateDataStorageCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Data Storage E2E Cluster Setup (ONCE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Build Data Storage Docker image
	fmt.Fprintln(writer, "🔨 Building Data Storage Docker image...")
	if err := buildDataStorageImage(writer); err != nil {
		return fmt.Errorf("failed to build Data Storage image: %w", err)
	}

	// 3. Load Data Storage image into Kind
	fmt.Fprintln(writer, "📦 Loading Data Storage image into Kind cluster...")
	if err := loadDataStorageImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Data Storage image: %w", err)
	}

	fmt.Fprintln(writer, "✅ Cluster ready - tests can now deploy services per-namespace")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeleteCluster deletes a Kind cluster
func DeleteCluster(clusterName string, writer io.Writer) error {
	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Failed to delete cluster: %s\n", output)
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	return nil
}

// DeployDataStorageTestServices deploys PostgreSQL, Redis, and Data Storage Service to a namespace
// This is used by E2E tests to create isolated test environments
func DeployDataStorageTestServices(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Deploying Data Storage Test Services in Namespace: %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Deploy PostgreSQL (V1.0: standard postgres, no pgvector)
	fmt.Fprintf(writer, "🚀 Deploying PostgreSQL...\n")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 3. Deploy Redis for DLQ
	fmt.Fprintf(writer, "🚀 Deploying Redis for DLQ...\n")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 4. Apply database migrations using shared migration library
	fmt.Fprintf(writer, "📋 Applying database migrations...\n")
	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Deploy Data Storage Service
	fmt.Fprintf(writer, "🚀 Deploying Data Storage Service...\n")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
	}

	// 6. Wait for all services ready
	fmt.Fprintf(writer, "⏳ Waiting for services to be ready...\n")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	fmt.Fprintf(writer, "✅ Data Storage test services ready in namespace %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// CleanupDataStorageTestNamespace deletes a test namespace and all resources
func CleanupDataStorageTestNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "🧹 Cleaning up namespace %s...\n", namespace)

	cmd := exec.Command("kubectl", "delete", "namespace", namespace,
		"--kubeconfig", kubeconfigPath,
		"--wait=true",
		"--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "⚠️  Failed to delete namespace: %s\n", output)
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	fmt.Fprintf(writer, "✅ Namespace %s deleted\n", namespace)
	return nil
}

func createTestNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"test": "datastorage-e2e",
			},
		},
	}

	_, err = clientset.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "AlreadyExists") {
			fmt.Fprintf(writer, "   Namespace %s already exists\n", namespace)
			return nil
		}
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	fmt.Fprintf(writer, "   ✅ Namespace %s created\n", namespace)
	return nil
}

func getKubernetesClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

func deployPostgreSQLInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create ConfigMap for init script
	initConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-init",
			Namespace: namespace,
		},
		Data: map[string]string{
			"init.sql": `-- V1.0: Standard PostgreSQL (no pgvector extension)

-- Grant permissions to slm_user
GRANT ALL PRIVILEGES ON DATABASE action_history TO slm_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;`,
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, initConfigMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL init ConfigMap: %w", err)
	}

	// 2. Create Secret for credentials
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-secret",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"POSTGRES_USER":     "slm_user",
			"POSTGRES_PASSWORD": "test_password",
			"POSTGRES_DB":       "action_history",
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL secret: %w", err)
	}

	// 3. Create Service (NodePort for direct access from host - eliminates port-forward instability)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "postgresql",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "postgresql",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					NodePort:   30432, // Mapped to localhost:5432 via Kind extraPortMappings
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "postgresql",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL service: %w", err)
	}

	// 4. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "postgresql",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "postgresql",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "postgresql",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgresql",
							Image: "postgres:16-alpine", // V1.0: standard postgres, no pgvector
							Ports: []corev1.ContainerPort{
								{
									Name:          "postgresql",
									ContainerPort: 5432,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "POSTGRES_USER",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_USER",
										},
									},
								},
								{
									Name: "POSTGRES_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_PASSWORD",
										},
									},
								},
								{
									Name: "POSTGRES_DB",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_DB",
										},
									},
								},
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "postgresql-data",
									MountPath: "/var/lib/postgresql/data",
								},
								{
									Name:      "postgresql-init",
									MountPath: "/docker-entrypoint-initdb.d",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "slm_user"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "slm_user"},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "postgresql-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "postgresql-init",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "postgresql-init",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL deployment: %w", err)
	}

	fmt.Fprintf(writer, "   ✅ PostgreSQL deployed (ConfigMap + Secret + Service + Deployment)\n")
	return nil
}

func deployRedisInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "redis",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "redis",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Redis service: %w", err)
	}

	// 2. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "redis",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "redis",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "redis",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "quay.io/jordigilh/redis:7-alpine",
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: 6379,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("200m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Redis deployment: %w", err)
	}

	fmt.Fprintf(writer, "   ✅ Redis deployed (Service + Deployment)\n")
	return nil
}

// ApplyMigrations is an exported wrapper for applying ALL migrations to a namespace.
// This is useful for re-applying migrations after PostgreSQL restarts (e.g., in DLQ tests).
//
// DEPRECATED: Use ApplyAllMigrations() for DS full schema, or ApplyAuditMigrations() for audit-only.
// This function is kept for backward compatibility.
func ApplyMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Delegate to shared migration library
	return ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
}

func deployDataStorageServiceInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create ConfigMap for service configuration
	configYAML := fmt.Sprintf(`service:
  name: data-storage
  metricsPort: 9090
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: postgresql.%s.svc.cluster.local
  port: 5432
  name: action_history
  user: slm_user
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: redis.%s.svc.cluster.local:6379
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json`, namespace, namespace)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.yaml": configYAML,
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage ConfigMap: %w", err)
	}

	// 2. Create Secret for database and Redis credentials
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage-secret",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"db-secrets.yaml": `username: slm_user
password: test_password`,
			"redis-secrets.yaml": `password: ""`,
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Secret: %w", err)
	}

	// 3. Create Service (NodePort for direct access from host - eliminates port-forward instability)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "datastorage",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					NodePort:   30081, // Mapped to localhost:8081 via Kind extraPortMappings
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "datastorage",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Service: %w", err)
	}

	// 4. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "datastorage",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datastorage",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "datastorage",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "datastorage",
							Image: "localhost/kubernaut-datastorage:e2e-test",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
								},
								{
									Name:          "metrics",
									ContainerPort: 9090,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "CONFIG_PATH",
									Value: "/etc/datastorage/config.yaml",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/datastorage",
									ReadOnly:  true,
								},
								{
									Name:      "secrets",
									MountPath: "/etc/datastorage/secrets",
									ReadOnly:  true,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "datastorage-config",
									},
								},
							},
						},
						{
							Name: "secrets",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "datastorage-secret",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Deployment: %w", err)
	}

	fmt.Fprintf(writer, "   ✅ Data Storage Service deployed (ConfigMap + Secret + Service + Deployment)\n")
	return nil
}

func waitForDataStorageServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// Wait for PostgreSQL pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for PostgreSQL pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=postgresql",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 60*time.Second, 2*time.Second).Should(BeTrue(), "PostgreSQL pod should be ready")
	fmt.Fprintf(writer, "   ✅ PostgreSQL pod ready\n")

	// Wait for Redis pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for Redis pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=redis",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 60*time.Second, 2*time.Second).Should(BeTrue(), "Redis pod should be ready")
	fmt.Fprintf(writer, "   ✅ Redis pod ready\n")

	// Wait for Data Storage Service pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for Data Storage Service pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=datastorage",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 60*time.Second, 2*time.Second).Should(BeTrue(), "Data Storage Service pod should be ready")
	fmt.Fprintf(writer, "   ✅ Data Storage Service pod ready\n")

	return nil
}

func createKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Check if cluster already exists
	checkCmd := exec.Command("kind", "get", "clusters")
	checkOutput, _ := checkCmd.CombinedOutput()
	if strings.Contains(string(checkOutput), clusterName) {
		fmt.Fprintln(writer, "  ⚠️  Cluster already exists, deleting...")
		DeleteCluster(clusterName, writer)
	}

	// Use external config file with extraPortMappings for NodePort access
	// This eliminates kubectl port-forward instability (like Gateway E2E tests)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}
	configPath := filepath.Join(workspaceRoot, "test", "infrastructure", "kind-datastorage-config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("kind config file not found: %s", configPath)
	}

	fmt.Fprintf(writer, "  📋 Using Kind config: %s\n", configPath)

	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Kind create output:\n%s\n", output)
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ Kind cluster created")

	// Export kubeconfig to specified path
	// Note: kind create --kubeconfig doesn't always work reliably, so we export explicitly
	kubeconfigCmd := exec.Command("kind", "get", "kubeconfig", "--name", clusterName)
	// Use Output() instead of CombinedOutput() to avoid capturing stderr
	// (stderr contains "enabling experimental podman provider" which breaks YAML parsing)
	kubeconfigOutput, err := kubeconfigCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Ensure directory exists
	kubeconfigDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// Write kubeconfig to file
	if err := os.WriteFile(kubeconfigPath, kubeconfigOutput, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	fmt.Fprintf(writer, "  ✅ Kubeconfig written to %s\n", kubeconfigPath)
	return nil
}

func buildDataStorageImage(writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Build Data Storage image using Podman (following Gateway pattern)
	buildCmd := exec.Command("podman", "build",
		"-t", "localhost/kubernaut-datastorage:e2e-test",
		"-f", "docker/datastorage-ubi9.Dockerfile",
		".")
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	fmt.Fprintln(writer, "   Data Storage image built: localhost/kubernaut-datastorage:e2e-test")
	return nil
}

func loadDataStorageImage(clusterName string, writer io.Writer) error {
	// Save image to tar (following Gateway pattern)
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-datastorage:e2e-test", "-o", "/tmp/datastorage-e2e.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/datastorage-e2e.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Clean up tar file
	_ = os.Remove("/tmp/datastorage-e2e.tar")

	fmt.Fprintln(writer, "   Data Storage image loaded into Kind cluster")
	return nil
}

// DataStorageInfrastructure manages the Data Storage Service test infrastructure
// This includes PostgreSQL, Redis, and the Data Storage Service itself
type DataStorageInfrastructure struct {
	PostgresContainer string
	RedisContainer    string
	ServiceContainer  string
	ConfigDir         string
	ServiceURL        string
	DB                *sql.DB
	RedisClient       *redis.Client
}

// DataStorageConfig contains configuration for the Data Storage Service
type DataStorageConfig struct {
	PostgresPort string // Default: "5433"
	RedisPort    string // Default: "6380"
	ServicePort  string // Default: "8085"
	DBName       string // Default: "action_history"
	DBUser       string // Default: "slm_user"
	DBPassword   string // Default: "test_password"
}

// DefaultDataStorageConfig returns default configuration
func DefaultDataStorageConfig() *DataStorageConfig {
	return &DataStorageConfig{
		PostgresPort: "5433",
		RedisPort:    "6380",
		ServicePort:  "8085",
		DBName:       "action_history",
		DBUser:       "slm_user",
		DBPassword:   "test_password",
	}
}

// StartDataStorageInfrastructure starts all Data Storage Service infrastructure
// Returns an infrastructure handle that can be used to stop the services
func StartDataStorageInfrastructure(cfg *DataStorageConfig, writer io.Writer) (*DataStorageInfrastructure, error) {
	if cfg == nil {
		cfg = DefaultDataStorageConfig()
	}

	infra := &DataStorageInfrastructure{
		PostgresContainer: "datastorage-postgres-test",
		RedisContainer:    "datastorage-redis-test",
		ServiceContainer:  "datastorage-service-test",
		ServiceURL:        fmt.Sprintf("http://localhost:%s", cfg.ServicePort),
	}

	fmt.Fprintln(writer, "🔧 Setting up Data Storage Service infrastructure (ADR-016: Podman)")

	// 1. Start PostgreSQL
	fmt.Fprintln(writer, "📦 Starting PostgreSQL container...")
	if err := startPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// 2. Start Redis
	fmt.Fprintln(writer, "📦 Starting Redis container...")
	if err := startRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start Redis: %w", err)
	}

	// 3. Connect to PostgreSQL
	fmt.Fprintln(writer, "🔌 Connecting to PostgreSQL...")
	if err := connectPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// 4. Apply migrations
	fmt.Fprintln(writer, "📋 Applying schema migrations...")
	if err := applyMigrations(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Connect to Redis
	fmt.Fprintln(writer, "🔌 Connecting to Redis...")
	if err := connectRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// 6. Create config files
	fmt.Fprintln(writer, "📝 Creating ADR-030 config files...")
	if err := createConfigFiles(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to create config files: %w", err)
	}

	// 7. Build Data Storage Service image
	fmt.Fprintln(writer, "🏗️  Building Data Storage Service image...")
	if err := buildDataStorageService(writer); err != nil {
		return nil, fmt.Errorf("failed to build service: %w", err)
	}

	// 8. Start Data Storage Service
	fmt.Fprintln(writer, "🚀 Starting Data Storage Service container...")
	if err := startDataStorageService(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start service: %w", err)
	}

	// 9. Wait for service to be ready
	fmt.Fprintln(writer, "⏳ Waiting for Data Storage Service to be ready...")
	if err := waitForServiceReady(infra, writer); err != nil {
		return nil, fmt.Errorf("service not ready: %w", err)
	}

	fmt.Fprintln(writer, "✅ Data Storage Service infrastructure ready!")
	return infra, nil
}

// StopDataStorageInfrastructure stops all Data Storage Service infrastructure
func (infra *DataStorageInfrastructure) Stop(writer io.Writer) {
	fmt.Fprintln(writer, "🧹 Cleaning up Data Storage Service infrastructure...")

	// Close connections
	if infra.DB != nil {
		infra.DB.Close()
	}
	if infra.RedisClient != nil {
		infra.RedisClient.Close()
	}

	// Stop and remove containers
	exec.Command("podman", "stop", infra.ServiceContainer).Run()
	exec.Command("podman", "rm", infra.ServiceContainer).Run()
	exec.Command("podman", "stop", infra.PostgresContainer).Run()
	exec.Command("podman", "rm", infra.PostgresContainer).Run()
	exec.Command("podman", "stop", infra.RedisContainer).Run()
	exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Remove config directory
	if infra.ConfigDir != "" {
		os.RemoveAll(infra.ConfigDir)
	}

	fmt.Fprintln(writer, "✅ Data Storage Service infrastructure cleanup complete")
}

// Helper functions

func startPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	exec.Command("podman", "stop", infra.PostgresContainer).Run()
	exec.Command("podman", "rm", infra.PostgresContainer).Run()

	// Start PostgreSQL (V1.0: standard postgres, no pgvector)
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.PostgresContainer,
		"-p", fmt.Sprintf("%s:5432", cfg.PostgresPort),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", cfg.DBName),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", cfg.DBUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", cfg.DBPassword),
		"postgres:16-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Failed to start PostgreSQL: %s\n", output)
		return fmt.Errorf("PostgreSQL container failed to start: %w", err)
	}

	// Wait for PostgreSQL ready
	fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL to be ready...")
	time.Sleep(3 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.PostgresContainer, "pg_isready", "-U", cfg.DBUser)
		return testCmd.Run()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

	fmt.Fprintln(writer, "  ✅ PostgreSQL started successfully")
	return nil
}

func startRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	exec.Command("podman", "stop", infra.RedisContainer).Run()
	exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Start Redis
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.RedisContainer,
		"-p", fmt.Sprintf("%s:6379", cfg.RedisPort),
		"quay.io/jordigilh/redis:7-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Failed to start Redis: %s\n", output)
		return fmt.Errorf("Redis container failed to start: %w", err)
	}

	// Wait for Redis ready
	time.Sleep(2 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.RedisContainer, "redis-cli", "ping")
		testOutput, err := testCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Redis not ready: %v, output: %s", err, string(testOutput))
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	fmt.Fprintln(writer, "  ✅ Redis started successfully")
	return nil
}

func connectPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	connStr := fmt.Sprintf("host=localhost port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.PostgresPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	var err error
	infra.DB, err = sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Wait for connection
	Eventually(func() error {
		return infra.DB.Ping()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be connectable")

	fmt.Fprintln(writer, "  ✅ PostgreSQL connection established")
	return nil
}

func applyMigrations(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Drop and recreate schema
	fmt.Fprintln(writer, "  🗑️  Dropping existing schema...")
	_, err := infra.DB.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	if err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	// V1.0: pgvector extension REMOVED (label-only architecture)
	// See: docs/handoff/RESPONSE_DS_PGVECTOR_CLEANUP_COMPLETE.md

	// Apply migrations (V1.0 label-only, no vector migrations)
	fmt.Fprintln(writer, "  📜 Applying V1.0 migrations (label-only, no embeddings)...")
	// V1.0 Migration List (label-only architecture, no embeddings)
	// Removed vector-dependent migrations per TRIAGE_DS_MIGRATION_DEPENDENCIES_V1.0.md:
	// - 005_vector_schema.sql (creates action_patterns with embedding vector)
	// - 007_add_context_column.sql (depends on 005)
	// - 008_context_api_compatibility.sql (adds embedding column)
	// - 009_update_vector_dimensions.sql (updates vector dimensions)
	// - 010_audit_write_api_phase1.sql (creates tables with vector columns)
	// - 011_rename_alert_to_signal.sql (depends on 010)
	// - 015_create_workflow_catalog_table.sql (creates workflows with embedding)
	// - 016_update_embedding_dimensions.sql (updates to 768 dimensions)
	migrations := []string{
		"001_initial_schema.sql",
		"002_fix_partitioning.sql",
		"003_stored_procedures.sql",
		"004_add_effectiveness_assessment_due.sql",
		"006_effectiveness_assessment.sql",
		"012_adr033_multidimensional_tracking.sql",
		"013_create_audit_events_table.sql",
		"017_add_workflow_schema_fields.sql",
		"018_rename_execution_bundle_to_container_image.sql",
		"019_uuid_primary_key.sql",
		"020_add_workflow_label_columns.sql", // DD-WORKFLOW-001 v1.6: custom_labels + detected_labels
		"1000_create_audit_events_partitions.sql",
	}

	// Find workspace root once (project root with go.mod)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	for _, migration := range migrations {
		// Use absolute path from project root (no relative path issues)
		migrationPath := filepath.Join(workspaceRoot, "migrations", migration)

		content, err := os.ReadFile(migrationPath)
		if err != nil {
			fmt.Fprintf(writer, "  ❌ Migration file not found at %s: %v\n", migrationPath, err)
			return fmt.Errorf("migration file %s not found at %s: %w", migration, migrationPath, err)
		}

		// Remove CONCURRENTLY keyword for test environment
		migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

		// Extract only the UP migration (ignore DOWN section)
		if strings.Contains(migrationSQL, "-- +goose Down") {
			parts := strings.Split(migrationSQL, "-- +goose Down")
			migrationSQL = parts[0]
		}

		_, err = infra.DB.Exec(migrationSQL)
		if err != nil {
			fmt.Fprintf(writer, "  ❌ Migration %s failed: %v\n", migration, err)
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}
		fmt.Fprintf(writer, "  ✅ Applied %s\n", migration)
	}

	// Grant permissions
	fmt.Fprintln(writer, "  🔐 Granting permissions...")
	_, err = infra.DB.Exec(`
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`)
	if err != nil {
		return fmt.Errorf("failed to grant permissions: %w", err)
	}

	// Wait for schema propagation
	fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL schema propagation (2s)...")
	time.Sleep(2 * time.Second)

	fmt.Fprintln(writer, "  ✅ All migrations applied successfully")
	return nil
}

func connectRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	infra.RedisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("localhost:%s", cfg.RedisPort),
		DB:   0,
	})

	// Verify connection
	err := infra.RedisClient.Ping(context.Background()).Err()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ Redis connection established")
	return nil
}

func createConfigFiles(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	var err error
	infra.ConfigDir, err = os.MkdirTemp("", "datastorage-config-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Get container IPs
	postgresIP := getContainerIP(infra.PostgresContainer)
	redisIP := getContainerIP(infra.RedisContainer)

	// Create config.yaml (ADR-030)
	configYAML := fmt.Sprintf(`
service:
  name: data-storage
  metricsPort: 9090
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: %s
  port: 5432
  name: %s
  user: %s
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: %s:6379
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json
`, postgresIP, cfg.DBName, cfg.DBUser, redisIP)

	configPath := filepath.Join(infra.ConfigDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	// Create database secrets file
	dbSecretsYAML := fmt.Sprintf(`
username: %s
password: %s
`, cfg.DBUser, cfg.DBPassword)
	dbSecretsPath := filepath.Join(infra.ConfigDir, "db-secrets.yaml")
	err = os.WriteFile(dbSecretsPath, []byte(dbSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write db-secrets.yaml: %w", err)
	}

	// Create Redis secrets file
	redisSecretsYAML := `password: ""` // Redis without auth in test
	redisSecretsPath := filepath.Join(infra.ConfigDir, "redis-secrets.yaml")
	err = os.WriteFile(redisSecretsPath, []byte(redisSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write redis-secrets.yaml: %w", err)
	}

	fmt.Fprintf(writer, "  ✅ Config files created in %s\n", infra.ConfigDir)
	return nil
}

func buildDataStorageService(writer io.Writer) error {
	// Find workspace root (go.mod location)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Cleanup any existing image
	exec.Command("podman", "rmi", "-f", "data-storage:test").Run()

	// Build image for ARM64 (local testing on Apple Silicon)
	buildCmd := exec.Command("podman", "build",
		"--build-arg", "GOARCH=arm64",
		"-t", "data-storage:test",
		"-f", "docker/data-storage.Dockerfile",
		".")
	buildCmd.Dir = workspaceRoot // Run from workspace root

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Build output:\n%s\n", string(output))
		return fmt.Errorf("failed to build Data Storage Service image: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ Data Storage Service image built successfully")
	return nil
}

// findWorkspaceRoot finds the workspace root by looking for go.mod
func findWorkspaceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up the directory tree looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

func startDataStorageService(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	exec.Command("podman", "stop", infra.ServiceContainer).Run()
	exec.Command("podman", "rm", infra.ServiceContainer).Run()

	// Mount config files (ADR-030)
	configMount := fmt.Sprintf("%s/config.yaml:/etc/datastorage/config.yaml:ro", infra.ConfigDir)
	secretsMount := fmt.Sprintf("%s:/etc/datastorage/secrets:ro", infra.ConfigDir)

	// Start service container with ADR-030 config
	startCmd := exec.Command("podman", "run", "-d",
		"--name", infra.ServiceContainer,
		"-p", fmt.Sprintf("%s:8080", cfg.ServicePort),
		"-v", configMount,
		"-v", secretsMount,
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"data-storage:test")

	output, err := startCmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(writer, "❌ Start output:\n%s\n", string(output))
		return fmt.Errorf("failed to start Data Storage Service container: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ Data Storage Service container started")
	return nil
}

func waitForServiceReady(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Wait up to 30 seconds for service to be ready
	var lastStatusCode int
	var lastError error

	Eventually(func() int {
		resp, err := http.Get(infra.ServiceURL + "/health")
		if err != nil {
			lastError = err
			lastStatusCode = 0
			fmt.Fprintf(writer, "    Health check attempt failed: %v\n", err)
			return 0
		}
		if resp == nil {
			lastStatusCode = 0
			return 0
		}
		defer resp.Body.Close()
		lastStatusCode = resp.StatusCode
		if lastStatusCode != 200 {
			fmt.Fprintf(writer, "    Health check returned status %d (expected 200)\n", lastStatusCode)
		}
		return lastStatusCode
	}, "30s", "1s").Should(Equal(200), "Data Storage Service should be healthy")

	// If we got here and status is not 200, print diagnostics
	if lastStatusCode != 200 {
		fmt.Fprintf(writer, "\n❌ Data Storage Service health check failed\n")
		fmt.Fprintf(writer, "  Last status code: %d\n", lastStatusCode)
		if lastError != nil {
			fmt.Fprintf(writer, "  Last error: %v\n", lastError)
		}

		// Print container logs for debugging
		logs, logErr := exec.Command("podman", "logs", "--tail", "200", infra.ServiceContainer).CombinedOutput()
		if logErr == nil {
			fmt.Fprintf(writer, "\n📋 Data Storage Service logs (last 200 lines):\n%s\n", string(logs))
		}

		// Check if container is running
		statusCmd := exec.Command("podman", "ps", "--filter", fmt.Sprintf("name=%s", infra.ServiceContainer), "--format", "{{.Status}}")
		statusOutput, _ := statusCmd.CombinedOutput()
		fmt.Fprintf(writer, "  Container status: %s\n", strings.TrimSpace(string(statusOutput)))
	}

	fmt.Fprintf(writer, "  ✅ Data Storage Service ready at %s\n", infra.ServiceURL)
	return nil
}

func getContainerIP(containerName string) string {
	cmd := exec.Command("podman", "inspect", "-f", "{{.NetworkSettings.IPAddress}}", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to get IP for container %s: %v", containerName, err))
	}
	ip := strings.TrimSpace(string(output))
	if ip == "" {
		panic(fmt.Sprintf("Container %s has no IP address", containerName))
	}
	return ip
}
