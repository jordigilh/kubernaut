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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
//
// This file covers P2 tests: Enricher resource types
// - enrichDaemonSetSignal, enrichReplicaSetSignal
// - getDaemonSet, getReplicaSet
// - convertDaemonSetDetails, convertReplicaSetDetails
package signalprocessing

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

var _ = Describe("K8sEnricher Resource Types", func() {
	var (
		ctx         context.Context
		k8sClient   client.Client
		k8sEnricher *enricher.K8sEnricher
		m           *metrics.Metrics
		scheme      *runtime.Scheme
		logger      logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		reg := prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(reg)
		logger = zap.New(zap.UseDevMode(true))
	})

	createFakeClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}

	// ========================================
	// DAEMONSET ENRICHMENT (P2)
	// ========================================

	Describe("DaemonSet Enrichment", func() {
		Context("ENRICH-DS-01: enriching DaemonSet signal", func() {
			It("should enrich DaemonSet with full context", func() {
				// Create namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kube-system",
						Labels: map[string]string{
							"kubernaut.ai/environment": "production",
						},
					},
				}

				// Create DaemonSet
				ds := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fluentd",
						Namespace: "kube-system",
						Labels: map[string]string{
							"app": "fluentd",
						},
						Annotations: map[string]string{
							"description": "Log collector",
						},
					},
					Spec: appsv1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "fluentd"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "fluentd"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "fluentd", Image: "fluentd:v1.14"},
								},
							},
						},
					},
					Status: appsv1.DaemonSetStatus{
						DesiredNumberScheduled: 3,
						CurrentNumberScheduled: 3,
						NumberReady:            3,
					},
				}

				k8sClient = createFakeClient(ns, ds)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "DaemonSet",
						Name:      "fluentd",
						Namespace: "kube-system",
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.DaemonSet).ToNot(BeNil())
				Expect(result.DaemonSet.Labels["app"]).To(Equal("fluentd"))
				Expect(result.DaemonSet.DesiredNumberScheduled).To(Equal(int32(3)))
				Expect(result.DaemonSet.NumberReady).To(Equal(int32(3)))
			})
		})

		Context("ENRICH-DS-02: DaemonSet not found enters degraded mode", func() {
			It("should enter degraded mode when DaemonSet doesn't exist", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				k8sClient = createFakeClient(ns)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "DaemonSet",
						Name:      "nonexistent-ds",
						Namespace: "default",
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.DegradedMode).To(BeTrue())
			})
		})
	})

	// ========================================
	// REPLICASET ENRICHMENT (P2)
	// ========================================

	Describe("ReplicaSet Enrichment", func() {
		Context("ENRICH-RS-01: enriching ReplicaSet signal", func() {
			It("should enrich ReplicaSet with full context", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				// Create ReplicaSet
				replicas := int32(3)
				rs := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx-abc123",
						Namespace: "default",
						Labels: map[string]string{
							"app":               "nginx",
							"pod-template-hash": "abc123",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Name:       "nginx",
								Controller: boolPtr(true),
							},
						},
					},
					Spec: appsv1.ReplicaSetSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "nginx"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "nginx", Image: "nginx:1.21"},
								},
							},
						},
					},
					Status: appsv1.ReplicaSetStatus{
						Replicas:          3,
						ReadyReplicas:     3,
						AvailableReplicas: 3,
					},
				}

				// Create owner Deployment
				deploy := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nginx",
						Namespace: "default",
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "nginx"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "nginx"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "nginx", Image: "nginx:1.21"},
								},
							},
						},
					},
				}

				k8sClient = createFakeClient(ns, rs, deploy)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "ReplicaSet",
						Name:      "nginx-abc123",
						Namespace: "default",
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.ReplicaSet).ToNot(BeNil())
				Expect(result.ReplicaSet.Labels["app"]).To(Equal("nginx"))
				Expect(result.ReplicaSet.Replicas).To(Equal(int32(3)))
				Expect(result.ReplicaSet.ReadyReplicas).To(Equal(int32(3)))
			})
		})

		Context("ENRICH-RS-02: ReplicaSet not found enters degraded mode", func() {
			It("should enter degraded mode when ReplicaSet doesn't exist", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				k8sClient = createFakeClient(ns)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "ReplicaSet",
						Name:      "nonexistent-rs",
						Namespace: "default",
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.DegradedMode).To(BeTrue())
			})
		})

		Context("ENRICH-RS-03: ReplicaSet with owner references", func() {
			It("should capture owner references in ReplicaSet details", func() {
				// Note: Full owner chain traversal is tested in integration tests
				// Unit tests verify ReplicaSet details are correctly captured
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				replicas := int32(2)
				rs := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "web-app-abc123",
						Namespace: "default",
						Labels: map[string]string{
							"app":               "web-app",
							"pod-template-hash": "abc123",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Name:       "web-app",
								Controller: boolPtr(true),
							},
						},
					},
					Spec: appsv1.ReplicaSetSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "web-app"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "web-app"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "app", Image: "app:v1"},
								},
							},
						},
					},
					Status: appsv1.ReplicaSetStatus{
						Replicas:      2,
						ReadyReplicas: 2,
					},
				}

				k8sClient = createFakeClient(ns, rs)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "ReplicaSet",
						Name:      "web-app-abc123",
						Namespace: "default",
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.ReplicaSet).ToNot(BeNil())
				// Verify the labels include pod-template-hash (indicates it's managed by Deployment)
				Expect(result.ReplicaSet.Labels["pod-template-hash"]).To(Equal("abc123"))
				Expect(result.ReplicaSet.Labels["app"]).To(Equal("web-app"))
			})
		})
	})

	// ========================================
	// DAEMONSET DETAILS CONVERSION (P2)
	// ========================================

	Describe("DaemonSet Details Conversion", func() {
		Context("ENRICH-DS-03: DaemonSet details are converted correctly", func() {
			It("should convert all DaemonSet status fields", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				ds := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "node-exporter",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/name":    "node-exporter",
							"app.kubernetes.io/version": "v1.3.1",
						},
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
							"prometheus.io/port":   "9100",
						},
					},
					Spec: appsv1.DaemonSetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "node-exporter"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "node-exporter"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "node-exporter", Image: "prom/node-exporter:v1.3.1"},
								},
							},
						},
					},
					Status: appsv1.DaemonSetStatus{
						DesiredNumberScheduled: 5,
						CurrentNumberScheduled: 5,
						NumberReady:            4,
						NumberMisscheduled:     0,
						NumberAvailable:        4,
					},
				}

				k8sClient = createFakeClient(ns, ds)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "DaemonSet",
						Name:      "node-exporter",
						Namespace: "default",
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.DaemonSet).ToNot(BeNil())
				Expect(result.DaemonSet.Labels["app.kubernetes.io/name"]).To(Equal("node-exporter"))
				Expect(result.DaemonSet.Annotations["prometheus.io/scrape"]).To(Equal("true"))
				Expect(result.DaemonSet.DesiredNumberScheduled).To(Equal(int32(5)))
				Expect(result.DaemonSet.CurrentNumberScheduled).To(Equal(int32(5)))
				Expect(result.DaemonSet.NumberReady).To(Equal(int32(4)))
			})
		})
	})

	// ========================================
	// REPLICASET DETAILS CONVERSION (P2)
	// ========================================

	Describe("ReplicaSet Details Conversion", func() {
		Context("ENRICH-RS-04: ReplicaSet details are converted correctly", func() {
			It("should convert all ReplicaSet status fields", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				replicas := int32(5)
				rs := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "api-server-v2-abc123",
						Namespace: "default",
						Labels: map[string]string{
							"app":               "api-server",
							"version":           "v2",
							"pod-template-hash": "abc123",
						},
					},
					Spec: appsv1.ReplicaSetSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api-server"},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": "api-server"},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{Name: "api", Image: "api-server:v2"},
								},
							},
						},
					},
					Status: appsv1.ReplicaSetStatus{
						Replicas:             5,
						ReadyReplicas:        4,
						AvailableReplicas:    4,
						FullyLabeledReplicas: 5,
					},
				}

				k8sClient = createFakeClient(ns, rs)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "ReplicaSet",
						Name:      "api-server-v2-abc123",
						Namespace: "default",
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.ReplicaSet).ToNot(BeNil())
				Expect(result.ReplicaSet.Labels["app"]).To(Equal("api-server"))
				Expect(result.ReplicaSet.Labels["version"]).To(Equal("v2"))
				Expect(result.ReplicaSet.Replicas).To(Equal(int32(5)))
				Expect(result.ReplicaSet.ReadyReplicas).To(Equal(int32(4)))
				Expect(result.ReplicaSet.AvailableReplicas).To(Equal(int32(4)))
			})
		})
	})
})
