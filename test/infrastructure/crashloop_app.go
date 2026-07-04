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

package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Issue #1542: Real CrashLoopBackOff Trigger (bad-config app)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Shared between the FullPipeline (fp-e2e-*) and Fleet (remote cluster) E2E
// lanes so both exercise the same real, deterministic CrashLoopBackOff signal
// and the same crashloop-config-fix-v1 workflow.
//
// The app is a busybox container that reads APP_MODE from a ConfigMap at
// startup. With the "broken" value it exits 1 immediately, causing the
// kubelet to restart it with exponential backoff until the pod reaches
// Waiting.Reason == "CrashLoopBackOff" and the kubelet emits a Warning event
// with Reason "BackOff". The kubernetes-event-exporter forwards that event's
// raw `.reason` field verbatim as NormalizedSignal.SignalName (BR-GATEWAY-111
// dumb-pipe passthrough), which matches the mock LLM's "crashloop" scenario
// keyword list ("crashloop", "backoff") — see
// test/services/mock-llm/scenarios/registry_default.go.
//
// The crashloop-config-fix-v1 remediate.sh (test/fixtures/workflows/
// crashloop-config-fix-job/remediate.sh) fixes the crash by patching this
// exact ConfigMap key back to CrashLoopAppConfigGoodValue and restarting the
// Deployment.

const (
	// CrashLoopAppName is the Deployment/ConfigMap-consumer name for the
	// bad-config crash fixture.
	CrashLoopAppName = "crashloop-app"

	// CrashLoopAppConfigMapName is the ConfigMap the app reads APP_MODE from.
	CrashLoopAppConfigMapName = "crashloop-app-config"

	// CrashLoopAppConfigKey is the ConfigMap key holding the app mode.
	CrashLoopAppConfigKey = "APP_MODE"

	// CrashLoopAppBadValue causes the app to exit 1 on startup (triggers CrashLoopBackOff).
	CrashLoopAppBadValue = "broken"

	// CrashLoopAppGoodValue is the value remediate.sh restores to fix the crash.
	CrashLoopAppGoodValue = "healthy"
)

// DeployCrashLoopConfigApp deploys a minimal busybox Deployment that reads
// APP_MODE from a ConfigMap and exits 1 immediately when the value is not
// CrashLoopAppGoodValue. Starting with CrashLoopAppBadValue guarantees a
// deterministic, fast CrashLoopBackOff (no memory pressure or timing races).
//
// Parameters:
//   - targetNamespace: Namespace with kubernaut.ai/managed=true label
//   - kubeconfigPath: Path to kubeconfig (hub or remote cluster)
//   - writer: Output writer for progress logging
func DeployCrashLoopConfigApp(ctx context.Context, targetNamespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🐛 Deploying %s (bad ConfigMap, will CrashLoopBackOff) in namespace %s...\n", CrashLoopAppName, targetNamespace)

	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %[2]s
  namespace: %[1]s
  labels:
    app: %[3]s
    kubernaut.ai/managed: "true"
data:
  %[4]s: "%[5]s"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[3]s
  namespace: %[1]s
  labels:
    app: %[3]s
    kubernaut.ai/managed: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %[3]s
  template:
    metadata:
      labels:
        app: %[3]s
        kubernaut.ai/managed: "true"
    spec:
      automountServiceAccountToken: false
      restartPolicy: Always
      containers:
      - name: %[3]s
        image: busybox:1.36
        imagePullPolicy: IfNotPresent
        envFrom:
        - configMapRef:
            name: %[2]s
        command: ["sh", "-c"]
        args:
          - |
            if [ "$%[4]s" != "%[6]s" ]; then
              echo "FATAL: invalid %[4]s='$%[4]s' (expected '%[6]s')"
              exit 1
            fi
            echo "%[4]s=%[6]s, running normally"
            sleep 3600
`, targetNamespace, CrashLoopAppConfigMapName, CrashLoopAppName, CrashLoopAppConfigKey, CrashLoopAppBadValue, CrashLoopAppGoodValue)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}
