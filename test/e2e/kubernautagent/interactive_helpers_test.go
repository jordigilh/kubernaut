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

package kubernautagent

import (
	"context"
	"crypto/sha256"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// rrOption configures optional overrides for createTestRemediationRequest.
type rrOption func(*remediationv1.RemediationRequestSpec)

// withSignalName overrides the default "E2ETestSignal" signal name on the RR.
func withSignalName(name string) rrOption {
	return func(spec *remediationv1.RemediationRequestSpec) {
		spec.SignalName = name
	}
}

// createTestRemediationRequest provisions a minimal RemediationRequest CRD in the
// Kind cluster so that the RRExistenceChecker (HARM-004) allows the session to start.
// The RR is created with the bare minimum fields required by the CRD validation schema.
func createTestRemediationRequest(testCtx context.Context, rrID string, opts ...rrOption) {
	GinkgoHelper()

	scheme := k8sruntime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(remediationv1.AddToScheme(scheme)).To(Succeed())

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	Expect(err).NotTo(HaveOccurred(), "build kubeconfig for RR creation")

	cli, err := ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred(), "create controller-runtime client for RR creation")

	fingerprint := fmt.Sprintf("%064x", sha256.Sum256([]byte("e2e-test-"+rrID)))

	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rrID,
			Namespace: sharedNamespace,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        "E2ETestSignal",
			Severity:          "warning",
			SignalType:        "alert",
			SignalSource:      "e2e-test",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "e2e-test-pod",
				Namespace: sharedNamespace,
			},
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),
		},
	}

	for _, o := range opts {
		o(&rr.Spec)
	}

	Expect(cli.Create(testCtx, rr)).To(Succeed(),
		"should create RemediationRequest %s for E2E test", rrID)
	GinkgoWriter.Printf("  📋 Created RR fixture: %s/%s (signal=%s)\n", sharedNamespace, rrID, rr.Spec.SignalName)
}
