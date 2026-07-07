#!/usr/bin/env bash
# Helm Chart Smoke Test Runner
# Authority: docs/testing/342/TEST_PLAN.md
# Output: TAP v13 (Test Anything Protocol)
#
# Usage:
#   ./scripts/helm-smoke-test.sh --platform kind --image-tag 1.0.1-rc1-arm64 --chart-path charts/kubernaut/
#
# This chart targets non-OCP (vanilla Kubernetes) deployments (Issue #1589); for OpenShift,
# use the Kubernaut Operator instead. Flows executed:
#   Kind: Flow A (production lifecycle with hook TLS) + Flow B (dev quick start)

set -uo pipefail

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
PLATFORM="kind"
IMAGE_TAG=""
IMAGE_REGISTRY=""
PULL_SECRET=""
CHART_PATH="charts/kubernaut/"
NAMESPACE="kubernaut-system"
TIMEOUT_PODS="300s"
# "hook" (default, Flow A + Flow B) or "cert-manager" (Flow C only). cert-manager
# mode requires cert-manager and exactly one ClusterIssuer already installed in
# the cluster as a prerequisite -- the chart auto-selects it via `lookup` during
# a real `helm install` (see tls.certManager.issuerRef in values.yaml), so this
# script does not need to pass --set tls.certManager.issuerRef.name explicitly.
TLS_MODE="hook"

# TAP state
TAP_COUNT=0
TAP_PASS=0
TAP_FAIL=0

# Port-forward PID tracking
PF_PID=""

# Rego policy temp files (created in setup_policy_files, cleaned in cleanup)
POLICY_AA_FILE=""
POLICY_SP_FILE=""

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case $1 in
    --platform)   PLATFORM="$2";   shift 2 ;;
    --image-tag)  IMAGE_TAG="$2";  shift 2 ;;
    --registry)   IMAGE_REGISTRY="$2"; shift 2 ;;
    --pull-secret) PULL_SECRET="$2"; shift 2 ;;
    --chart-path) CHART_PATH="$2"; shift 2 ;;
    --namespace)  NAMESPACE="$2";  shift 2 ;;
    --timeout)    TIMEOUT_PODS="$2"; shift 2 ;;
    --tls-mode)   TLS_MODE="$2";   shift 2 ;;
    -h|--help)
      echo "Usage: $0 --platform kind --image-tag TAG --chart-path PATH [--registry REGISTRY] [--pull-secret NAME] [--tls-mode hook|cert-manager]"
      echo ""
      echo "Options:"
      echo "  --platform    Target platform: kind (default: kind)"
      echo "  --image-tag   Container image tag (required)"
      echo "  --registry    Container image registry (overrides global.image.registry)"
      echo "  --pull-secret Kubernetes docker-registry secret name for private registries"
      echo "  --chart-path  Path to chart directory (default: charts/kubernaut/)"
      echo "  --namespace   Kubernetes namespace (default: kubernaut-system)"
      echo "  --timeout     Pod readiness timeout (default: 300s)"
      echo "  --tls-mode    TLS mode to test: hook (Flow A+B, default) or cert-manager (Flow C)."
      echo "                cert-manager requires cert-manager + one ClusterIssuer pre-installed."
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [[ -z "$IMAGE_TAG" ]]; then
  echo "Error: --image-tag is required"
  exit 1
fi

if [[ "$TLS_MODE" != "hook" && "$TLS_MODE" != "cert-manager" ]]; then
  echo "Error: --tls-mode must be 'hook' or 'cert-manager', got '${TLS_MODE}'"
  exit 1
fi

# ---------------------------------------------------------------------------
# TAP helpers
# ---------------------------------------------------------------------------
tap_header() {
  echo "TAP version 13"
}

tap_ok() {
  local desc="$1"
  TAP_COUNT=$((TAP_COUNT + 1))
  TAP_PASS=$((TAP_PASS + 1))
  local line="ok ${TAP_COUNT} - ${desc}"
  echo "$line"
}

tap_not_ok() {
  local desc="$1"
  local diag="${2:-}"
  TAP_COUNT=$((TAP_COUNT + 1))
  TAP_FAIL=$((TAP_FAIL + 1))
  local line="not ok ${TAP_COUNT} - ${desc}"
  echo "$line"
  if [[ -n "$diag" ]]; then
    echo "  ---"
    echo "  message: ${diag}"
    echo "  ..."
  fi
}

tap_footer() {
  echo "1..${TAP_COUNT}"
  echo "# pass ${TAP_PASS}"
  echo "# fail ${TAP_FAIL}"
}

# ---------------------------------------------------------------------------
# Assertion helpers
# ---------------------------------------------------------------------------
assert_exit_code() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    tap_ok "$desc"
    return 0
  else
    tap_not_ok "$desc" "Command failed: $*"
    return 1
  fi
}

dump_pod_diagnostics() {
  local ns="$1"
  echo "# ── Must-gather: pod diagnostics (namespace: ${ns}) ──"
  echo "# --- kubectl get pods -o wide ---"
  kubectl get pods -n "$ns" -o wide 2>&1 || true
  echo ""
  echo "# --- kubectl get events --sort-by=.lastTimestamp ---"
  kubectl get events -n "$ns" --sort-by='.lastTimestamp' 2>&1 | tail -40 || true
  echo ""
  local pod
  for pod in $(kubectl get pods -n "$ns" --no-headers 2>/dev/null | grep -v "Running" | awk '{print $1}'); do
    echo "# --- kubectl describe pod/${pod} ---"
    kubectl describe pod/"$pod" -n "$ns" 2>&1 | tail -30 || true
    echo ""
  done
}

# ---------------------------------------------------------------------------
# Must-gather: comprehensive diagnostics archive for CI triage
# ---------------------------------------------------------------------------
MUST_GATHER_DIR=""

must_gather() {
  local ns="${1:-$NAMESPACE}"
  local trigger="${2:-manual}"
  local ts
  ts=$(date -u +%Y%m%d-%H%M%S)
  MUST_GATHER_DIR="/tmp/must-gather-helm-smoke-${ts}"
  mkdir -p "$MUST_GATHER_DIR"

  echo "# ══════════════════════════════════════════════════════════"
  echo "# MUST-GATHER: Collecting diagnostics (trigger: ${trigger})"
  echo "# Output: ${MUST_GATHER_DIR}"
  echo "# ══════════════════════════════════════════════════════════"

  # --- Cluster-level ---
  echo "# [1/9] Cluster info"
  kubectl cluster-info                         > "$MUST_GATHER_DIR/cluster-info.txt" 2>&1 || true
  kubectl get nodes -o wide                    > "$MUST_GATHER_DIR/nodes.txt" 2>&1 || true
  kubectl top nodes                            >> "$MUST_GATHER_DIR/nodes.txt" 2>&1 || true

  # --- Helm state ---
  echo "# [2/9] Helm release state"
  helm list -n "$ns" -a                        > "$MUST_GATHER_DIR/helm-list.txt" 2>&1 || true
  helm status kubernaut -n "$ns"               > "$MUST_GATHER_DIR/helm-status.txt" 2>&1 || true
  helm history kubernaut -n "$ns"              > "$MUST_GATHER_DIR/helm-history.txt" 2>&1 || true

  # --- Namespace resources ---
  echo "# [3/9] Namespace resources"
  kubectl get all -n "$ns" -o wide             > "$MUST_GATHER_DIR/all-resources.txt" 2>&1 || true
  kubectl get pods -n "$ns" -o yaml            > "$MUST_GATHER_DIR/pods.yaml" 2>&1 || true
  kubectl get jobs -n "$ns" -o wide            > "$MUST_GATHER_DIR/jobs.txt" 2>&1 || true
  kubectl get pvc -n "$ns" -o wide             > "$MUST_GATHER_DIR/pvcs.txt" 2>&1 || true
  kubectl get endpoints -n "$ns"               > "$MUST_GATHER_DIR/endpoints.txt" 2>&1 || true

  # --- Secrets inventory (names + keys only, no values) ---
  echo "# [4/9] Secrets inventory"
  kubectl get secrets -n "$ns" -o custom-columns='NAME:.metadata.name,TYPE:.type,KEYS:.data' \
    --no-headers 2>/dev/null | while IFS= read -r line; do
    local sname stype skeys
    sname=$(echo "$line" | awk '{print $1}')
    stype=$(echo "$line" | awk '{print $2}')
    skeys=$(kubectl get secret "$sname" -n "$ns" -o jsonpath='{.data}' 2>/dev/null | \
            python3 -c "import sys,json; print(','.join(json.load(sys.stdin).keys()))" 2>/dev/null || echo "?")
    echo "$sname  type=$stype  keys=[$skeys]"
  done > "$MUST_GATHER_DIR/secrets-inventory.txt" 2>&1 || true

  # --- Events (full, not truncated) ---
  echo "# [5/9] Events"
  kubectl get events -n "$ns" --sort-by='.lastTimestamp' \
    -o custom-columns='LAST:.lastTimestamp,TYPE:.type,REASON:.reason,OBJECT:.involvedObject.name,MESSAGE:.message' \
    > "$MUST_GATHER_DIR/events.txt" 2>&1 || true

  # --- Pod logs (all containers, including init + previous) ---
  echo "# [6/9] Pod logs"
  mkdir -p "$MUST_GATHER_DIR/logs"
  local pod
  for pod in $(kubectl get pods -n "$ns" --no-headers 2>/dev/null | awk '{print $1}'); do
    kubectl logs "$pod" -n "$ns" --all-containers --prefix \
      > "$MUST_GATHER_DIR/logs/${pod}.log" 2>&1 || true
    kubectl logs "$pod" -n "$ns" --all-containers --prefix --previous \
      > "$MUST_GATHER_DIR/logs/${pod}.previous.log" 2>/dev/null || true
    # Remove empty previous log files
    [[ -s "$MUST_GATHER_DIR/logs/${pod}.previous.log" ]] || rm -f "$MUST_GATHER_DIR/logs/${pod}.previous.log"
  done

  # --- Describe non-healthy pods ---
  echo "# [7/9] Pod descriptions (non-Running)"
  mkdir -p "$MUST_GATHER_DIR/describe"
  for pod in $(kubectl get pods -n "$ns" --no-headers 2>/dev/null | grep -vE '1/1.*Running|2/2.*Running|Completed' | awk '{print $1}'); do
    kubectl describe pod "$pod" -n "$ns" \
      > "$MUST_GATHER_DIR/describe/${pod}.txt" 2>&1 || true
  done

  # --- Webhook configurations ---
  echo "# [8/9] Webhook configurations"
  kubectl get mutatingwebhookconfigurations authwebhook-mutating -o yaml \
    > "$MUST_GATHER_DIR/mutating-webhook.yaml" 2>&1 || true
  kubectl get validatingwebhookconfigurations authwebhook-validating -o yaml \
    > "$MUST_GATHER_DIR/validating-webhook.yaml" 2>&1 || true

  # --- Hook job diagnostics ---
  echo "# [9/9] Hook jobs"
  for job in $(kubectl get jobs -n "$ns" --no-headers 2>/dev/null | awk '{print $1}'); do
    kubectl describe job "$job" -n "$ns" \
      > "$MUST_GATHER_DIR/describe/job-${job}.txt" 2>&1 || true
    for jpod in $(kubectl get pods -n "$ns" -l "job-name=$job" --no-headers 2>/dev/null | awk '{print $1}'); do
      kubectl logs "$jpod" -n "$ns" --all-containers \
        > "$MUST_GATHER_DIR/logs/job-${jpod}.log" 2>&1 || true
    done
  done

  # --- Archive ---
  local archive="/tmp/must-gather-helm-smoke-${ts}.tar.gz"
  tar -czf "$archive" -C /tmp "must-gather-helm-smoke-${ts}" 2>/dev/null || true

  echo "# ──────────────────────────────────────────────────────────"
  echo "# MUST-GATHER COMPLETE: ${archive}"
  echo "# Files collected: $(find "$MUST_GATHER_DIR" -type f | wc -l)"
  echo "# Archive size: $(du -sh "$archive" 2>/dev/null | awk '{print $1}')"
  echo "# ──────────────────────────────────────────────────────────"

  # Print summary to TAP output for quick triage in CI logs
  echo "#"
  echo "# === Quick Triage Summary ==="
  echo "# Pod Status:"
  kubectl get pods -n "$ns" --no-headers 2>/dev/null | awk '{printf "#   %-55s %s %s\n", $1, $2, $3}' || true
  echo "#"
  echo "# Non-Running Pod Events:"
  for pod in $(kubectl get pods -n "$ns" --no-headers 2>/dev/null | grep -vE '1/1.*Running|2/2.*Running|Completed' | awk '{print $1}'); do
    echo "#   ${pod}:"
    kubectl get events -n "$ns" --field-selector "involvedObject.name=$pod" \
      --sort-by='.lastTimestamp' --no-headers 2>/dev/null | tail -3 | \
      awk '{printf "#     %s %s %s\n", $1, $4, substr($0, index($0,$6))}' || true
  done
  echo "#"
  echo "# Recent Warning Events:"
  kubectl get events -n "$ns" --field-selector type=Warning \
    --sort-by='.lastTimestamp' --no-headers 2>/dev/null | tail -10 | \
    awk '{printf "#   %-20s %-15s %s\n", $4, $5, substr($0, index($0,$6))}' || true
  echo "#"
  echo "# Helm Release:"
  helm status kubernaut -n "$ns" --short 2>/dev/null | head -3 | sed 's/^/#   /' || echo "#   (no release found)"
  echo "#"
}

assert_pods_ready() {
  local expected_count="$1"
  local desc="${2:-ST-CHART-VERIFY-001: ${expected_count} pods reach 1/1 Running}"
  local ns="${3:-$NAMESPACE}"

  if ! kubectl wait --for=condition=Ready pod --all -n "$ns" --timeout="$TIMEOUT_PODS" >/dev/null 2>&1; then
    local status
    status=$(kubectl get pods -n "$ns" --no-headers 2>&1)
    tap_not_ok "$desc" "Timeout waiting for pods. Current state: ${status}"
    must_gather "$ns" "pods-not-ready"
    return 1
  fi

  local actual
  actual=$(kubectl get pods -n "$ns" --no-headers 2>/dev/null | grep -c "Running" || true)
  if [[ "$actual" -eq "$expected_count" ]]; then
    tap_ok "$desc"
    return 0
  else
    local status
    status=$(kubectl get pods -n "$ns" --no-headers 2>&1)
    tap_not_ok "$desc" "Expected ${expected_count} Running pods, got ${actual}. State: ${status}"
    must_gather "$ns" "pod-count-mismatch"
    return 1
  fi
}

assert_resource_exists() {
  local resource="$1"
  local name="$2"
  local ns="${3:-$NAMESPACE}"
  local desc="$4"

  if kubectl get "$resource" "$name" -n "$ns" >/dev/null 2>&1; then
    tap_ok "$desc"
    return 0
  else
    tap_not_ok "$desc" "${resource}/${name} not found in namespace ${ns}"
    return 1
  fi
}

assert_port_forward_responds() {
  local svc="$1"
  local local_port="$2"
  local path="$3"
  local desc="$4"
  local ns="${5:-$NAMESPACE}"
  local remote_port="${6:-8080}"

  cleanup_port_forward

  kubectl port-forward -n "$ns" "svc/${svc}" "${local_port}:${remote_port}" >/dev/null 2>&1 &
  PF_PID=$!
  sleep 3

  local http_code
  http_code=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:${local_port}${path}" 2>/dev/null || echo "000")

  cleanup_port_forward

  if [[ "$http_code" =~ ^2[0-9][0-9]$ ]]; then
    tap_ok "$desc"
    return 0
  else
    tap_not_ok "$desc" "HTTP ${http_code} from svc/${svc}:${remote_port}${path}"
    return 1
  fi
}

cleanup_port_forward() {
  if [[ -n "$PF_PID" ]] && kill -0 "$PF_PID" 2>/dev/null; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
  fi
  PF_PID=""
}

# ---------------------------------------------------------------------------
# Cleanup helper
# ---------------------------------------------------------------------------
full_cleanup() {
  local ns="${1:-$NAMESPACE}"
  echo "# Cleaning up namespace ${ns}..."

  # Delete CRDs BEFORE helm uninstall so controllers are still alive to process
  # finalizers on CRs (aianalyses, workflowexecutions). Reversing this order causes
  # CRD deletion to hang indefinitely when finalizer-bearing CRs can't be reconciled.
  kubectl delete -f "${CHART_PATH}/crds/" --timeout=60s 2>/dev/null || true

  helm uninstall kubernaut -n "$ns" --no-hooks --timeout 2m 2>/dev/null || true
  kubectl delete jobs --all -n "$ns" 2>/dev/null || true
  kubectl delete pods --all -n "$ns" --force --grace-period=0 2>/dev/null || true
  kubectl delete pvc --all -n "$ns" 2>/dev/null || true
  kubectl delete secret --all -n "$ns" 2>/dev/null || true
  kubectl delete ns "$ns" --ignore-not-found --timeout=60s 2>/dev/null || true
  kubectl delete ns kubernaut-workflows --ignore-not-found --timeout=60s 2>/dev/null || true
  sleep 5
}

# ---------------------------------------------------------------------------
# Platform-specific install flags
# ---------------------------------------------------------------------------
# Note: networkPolicies.apiServerCIDR(s)/apiServerPort are intentionally NOT
# set here. The chart auto-discovers the kube-apiserver's real backend
# endpoint IP(s)/port via `lookup` against the live "kubernetes" Endpoints
# object during a real helm install/upgrade (see
# kubernaut.np.apiServerPeers/apiServerPort in _helpers.tpl) -- this smoke
# test exercises that auto-discovery path rather than working around it.
# See PR #1571 for the investigation trail that led here (round 3 initially
# discovered the endpoint in bash and passed it via --set; that approach was
# superseded once the chart learned to discover it itself).
common_install_flags() {
  local flags=""
  flags+=" --set global.image.tag=${IMAGE_TAG}"
  if [[ -n "$IMAGE_REGISTRY" ]]; then
    flags+=" --set global.image.registry=${IMAGE_REGISTRY}"
    flags+=" --set global.image.namespace="
  fi
  if [[ -n "$PULL_SECRET" ]]; then
    flags+=" --set global.imagePullSecrets[0].name=${PULL_SECRET}"
  fi
  flags+=" --set monitoring.prometheus.enabled=false"
  flags+=" --set monitoring.alertManager.enabled=false"
  if [[ "$PLATFORM" == "kind" ]]; then
    flags+=" --set global.image.pullPolicy=IfNotPresent"
  fi
  echo "$flags"
}

tls_flags() {
  echo "--set tls.mode=hook"
}

setup_policy_files() {
  POLICY_AA_FILE=$(mktemp)
  POLICY_SP_FILE=$(mktemp)
  cat > "$POLICY_AA_FILE" <<'REGOEOF'
package aianalysis.approval
import rego.v1
default allow := false
allow if { input.environment != "production" }
REGOEOF
  cat > "$POLICY_SP_FILE" <<'REGOEOF'
package signalprocessing
import rego.v1
default severity := "info"
severity := "high" if { input.labels.severity == "critical" }
REGOEOF
}

cleanup_policy_files() {
  rm -f "$POLICY_AA_FILE" "$POLICY_SP_FILE"
}

policy_flags() {
  echo "--set-file aianalysis.policies.content=$POLICY_AA_FILE"
  echo "--set-file signalprocessing.policies.content=$POLICY_SP_FILE"
}

production_secret_flags() {
  echo "--set postgresql.auth.existingSecret=kubernaut-pg-credentials"
  echo "--set valkey.existingSecret=kubernaut-valkey-credentials"
  echo "--set kubernautAgent.llm.provider=openai"
  echo "--set kubernautAgent.llm.model=gpt-4o"
  echo "--set kubernautAgent.llm.credentialsSecretName=kubernaut-llm-credentials"
  echo "--set gateway.auth.signalSources[0].name=alertmanager"
  echo "--set gateway.auth.signalSources[0].serviceAccount=alertmanager-kube-prometheus-stack-alertmanager"
  echo "--set gateway.auth.signalSources[0].namespace=monitoring"
  policy_flags
}

# ---------------------------------------------------------------------------
# Scenario implementations
# ---------------------------------------------------------------------------

run_pre_001() {
  local desc="ST-CHART-PRE-001: Install CRDs"
  if kubectl apply --server-side --force-conflicts -f "${CHART_PATH}/crds/" >/dev/null 2>&1; then
    local count
    count=$(kubectl get crds 2>/dev/null | grep -c "kubernaut.ai" || true)
    if [[ "$count" -eq 10 ]]; then
      tap_ok "$desc (10 CRDs created)"
    else
      tap_not_ok "$desc" "Expected 10 CRDs, found ${count}"
    fi
  else
    tap_not_ok "$desc" "kubectl apply failed"
  fi
}

run_pre_002() {
  local desc="ST-CHART-PRE-002: Create namespace"
  assert_exit_code "$desc" kubectl create namespace "$NAMESPACE"
}

run_pre_003() {
  local desc="ST-CHART-PRE-003: Provision secrets"
  local pass=true
  local test_password="smoke-test-pass"

  # Consolidated PostgreSQL + DataStorage secret (#557)
  kubectl create secret generic kubernaut-pg-credentials \
    --from-literal=POSTGRES_USER=slm_user \
    --from-literal=POSTGRES_PASSWORD="$test_password" \
    --from-literal=POSTGRES_DB=action_history \
    --from-literal="db-secrets.yaml=$(printf 'username: slm_user\npassword: %s' "$test_password")" \
    -n "$NAMESPACE" >/dev/null 2>&1 || pass=false

  kubectl create secret generic kubernaut-valkey-credentials \
    --from-literal="valkey-secrets.yaml=password: \"${test_password}\"" \
    -n "$NAMESPACE" >/dev/null 2>&1 || pass=false

  kubectl create secret generic kubernaut-llm-credentials --from-literal=OPENAI_API_KEY=sk-smoke-test-placeholder -n "$NAMESPACE" >/dev/null 2>&1 || pass=false # pre-commit:allow-sensitive

  local secret_count=3
  if [[ -n "$PULL_SECRET" && -n "${PULL_SECRET_SERVER:-}" ]]; then
    kubectl create secret docker-registry "$PULL_SECRET" \
      --docker-server="$PULL_SECRET_SERVER" \
      --docker-username="${PULL_SECRET_USER:-}" \
      --docker-password="${PULL_SECRET_TOKEN:-}" \
      -n "$NAMESPACE" >/dev/null 2>&1 || pass=false
    secret_count=4
  fi

  if $pass; then
    tap_ok "$desc (${secret_count} secrets created)"
  else
    tap_not_ok "$desc" "One or more secret creation commands failed"
  fi
}

run_pre_004() {
  local desc="ST-CHART-PRE-004: Provision TLS certificates"
  local tmpdir
  tmpdir=$(mktemp -d)
  local service="authwebhook"

  openssl genrsa -out "$tmpdir/ca.key" 2048 2>/dev/null
  openssl req -new -x509 -days 365 -key "$tmpdir/ca.key" \
    -out "$tmpdir/ca.crt" -subj "/CN=authwebhook-ca" 2>/dev/null

  openssl genrsa -out "$tmpdir/tls.key" 2048 2>/dev/null
  openssl req -new -key "$tmpdir/tls.key" \
    -out "$tmpdir/tls.csr" \
    -subj "/CN=${service}.${NAMESPACE}.svc" \
    -addext "subjectAltName=DNS:${service},DNS:${service}.${NAMESPACE},DNS:${service}.${NAMESPACE}.svc,DNS:${service}.${NAMESPACE}.svc.cluster.local" \
    2>/dev/null

  printf "subjectAltName=DNS:%s,DNS:%s.%s,DNS:%s.%s.svc,DNS:%s.%s.svc.cluster.local" \
    "$service" "$service" "$NAMESPACE" "$service" "$NAMESPACE" "$service" "$NAMESPACE" \
    > "$tmpdir/san.cnf"

  openssl x509 -req -in "$tmpdir/tls.csr" \
    -CA "$tmpdir/ca.crt" -CAkey "$tmpdir/ca.key" -CAcreateserial \
    -out "$tmpdir/tls.crt" -days 365 -extfile "$tmpdir/san.cnf" 2>/dev/null

  local pass=true
  kubectl create secret tls authwebhook-tls \
    --cert="$tmpdir/tls.crt" --key="$tmpdir/tls.key" \
    -n "$NAMESPACE" >/dev/null 2>&1 || pass=false

  local ca_b64
  ca_b64=$(base64 < "$tmpdir/ca.crt" | tr -d '\n')
  kubectl patch secret authwebhook-tls -n "$NAMESPACE" \
    -p "{\"data\":{\"ca.crt\":\"$ca_b64\"}}" >/dev/null 2>&1 || pass=false

  rm -rf "$tmpdir"

  if $pass; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "TLS cert generation or resource creation failed"
  fi
}

run_inst_001() {
  local desc="ST-CHART-INST-001: Production install"
  local flags
  flags="$(common_install_flags) $(tls_flags)"

  # shellcheck disable=SC2046
  if helm install kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" \
    $(production_secret_flags) \
    $flags \
    --timeout 5m >/dev/null; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "helm install failed"
    return 1
  fi
}

run_inst_003() {
  local desc="ST-CHART-INST-003: Dev quick start"
  local flags
  flags="$(common_install_flags) $(tls_flags)"

  # shellcheck disable=SC2046
  if helm install kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" --create-namespace \
    $(production_secret_flags) \
    $flags \
    --timeout 5m >/dev/null; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "helm install failed"
    return 1
  fi
}

run_verify_001() {
  assert_pods_ready 13
}

run_verify_002() {
  assert_port_forward_responds \
    "kubernaut-agent" 8081 "/healthz" \
    "ST-CHART-VERIFY-002: Kubernaut Agent health endpoint" \
    "$NAMESPACE" 8081
}

run_verify_003() {
  assert_port_forward_responds \
    "data-storage-service" 8081 "/readyz" \
    "ST-CHART-VERIFY-003: DataStorage health endpoint" \
    "$NAMESPACE" 8081
}

run_verify_np() {
  local desc_base="ST-CHART-VERIFY-NP"
  local ns="${1:-$NAMESPACE}"
  local np_installed
  np_installed=$(kubectl get networkpolicies -n "$ns" --no-headers 2>/dev/null | wc -l | tr -d ' ')

  if [[ "$np_installed" -ge 10 ]]; then
    tap_ok "${desc_base}-001: ${np_installed} NetworkPolicies deployed in cluster"
  else
    tap_not_ok "${desc_base}-001: NetworkPolicies deployed" \
      "Expected >= 10, found ${np_installed}"
    return 1
  fi

  # Spot-check: AuthWebhook ingress on port 9443 (F-2)
  local aw_port
  aw_port=$(kubectl get networkpolicy -n "$ns" -l app=authwebhook \
    -o jsonpath='{.items[0].spec.ingress[0].ports[0].port}' 2>/dev/null || echo "")
  if [[ "$aw_port" == "9443" ]]; then
    tap_ok "${desc_base}-002: AuthWebhook ingress port is 9443 (pod port, not service port)"
  else
    tap_not_ok "${desc_base}-002: AuthWebhook ingress port" \
      "Expected 9443, got ${aw_port}"
  fi

  # Spot-check: Notification dual-label selector (F-1)
  local nt_labels
  nt_labels=$(kubectl get networkpolicy -n "$ns" -l app=notification-controller \
    -o jsonpath='{.items[0].spec.podSelector.matchLabels}' 2>/dev/null || echo "")
  if grep -q "controller-manager" <<< "$nt_labels"; then
    tap_ok "${desc_base}-003: Notification podSelector includes control-plane label (F-1)"
  else
    tap_not_ok "${desc_base}-003: Notification dual-label selector" \
      "Missing control-plane: controller-manager in ${nt_labels}"
  fi
}

run_tls_patch() {
  local desc="ST-CHART-TLS-PATCH: Patch webhooks with CA bundle (manual mode)"
  local ca_b64
  ca_b64=$(kubectl get secret authwebhook-tls -n "$NAMESPACE" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || echo "")
  if [[ -z "$ca_b64" ]]; then
    tap_not_ok "$desc" "ca.crt key not found in authwebhook-tls Secret"
    return 1
  fi

  local pass=true
  for wh_kind in mutatingwebhookconfigurations validatingwebhookconfigurations; do
    local wh_name
    case "$wh_kind" in
      mutating*)   wh_name="authwebhook-mutating" ;;
      validating*) wh_name="authwebhook-validating" ;;
    esac

    local count
    count=$(kubectl get "$wh_kind" "$wh_name" \
      -o jsonpath='{.webhooks[*].name}' 2>/dev/null | wc -w || echo "0")
    count=$((count + 0))

    local patch="["
    local i=0
    while [[ "$i" -lt "$count" ]]; do
      [[ "$i" -gt 0 ]] && patch="${patch},"
      patch="${patch}{\"op\":\"add\",\"path\":\"/webhooks/${i}/clientConfig/caBundle\",\"value\":\"${ca_b64}\"}"
      i=$((i + 1))
    done
    patch="${patch}]"

    kubectl patch "$wh_kind" "$wh_name" --type='json' -p "$patch" >/dev/null 2>&1 || pass=false
  done

  if $pass; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "Failed to patch one or more webhook configurations"
  fi
}

run_tls_001() {
  local pass=true
  assert_resource_exists secret authwebhook-tls "$NAMESPACE" \
    "ST-CHART-TLS-001a: authwebhook-tls Secret exists" || pass=false

  local ca_key
  ca_key=$(kubectl get secret authwebhook-tls -n "$NAMESPACE" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || echo "")
  if [[ -n "$ca_key" ]]; then
    tap_ok "ST-CHART-TLS-001b: authwebhook-tls Secret contains ca.crt key"
  else
    tap_not_ok "ST-CHART-TLS-001b: authwebhook-tls Secret contains ca.crt key" "ca.crt key missing"
    pass=false
  fi

  local cabundle
  cabundle=$(kubectl get mutatingwebhookconfigurations authwebhook-mutating \
    -o jsonpath='{.webhooks[0].clientConfig.caBundle}' 2>/dev/null || echo "")
  if [[ -n "$cabundle" ]]; then
    tap_ok "ST-CHART-TLS-001c: Webhook caBundle is non-empty"
  else
    tap_not_ok "ST-CHART-TLS-001c: Webhook caBundle is non-empty" "caBundle is empty or webhook not found"
  fi
}

# Issue #753: Inter-service TLS assertions (mandatory TLS).
run_tls_interservice() {
  local pass=true

  for secret_name in datastorage-tls gateway-tls kubernautagent-tls; do
    assert_resource_exists secret "$secret_name" "$NAMESPACE" \
      "ST-TLS-INTERSERVICE-001: ${secret_name} Secret exists" || pass=false

    local tls_crt
    tls_crt=$(kubectl get secret "$secret_name" -n "$NAMESPACE" \
      -o jsonpath='{.data.tls\.crt}' 2>/dev/null || echo "")
    if [[ -n "$tls_crt" ]]; then
      tap_ok "ST-TLS-INTERSERVICE-002: ${secret_name} has tls.crt"
    else
      tap_not_ok "ST-TLS-INTERSERVICE-002: ${secret_name} has tls.crt" "tls.crt missing"
      pass=false
    fi
  done

  assert_resource_exists configmap inter-service-ca "$NAMESPACE" \
    "ST-TLS-INTERSERVICE-003: inter-service-ca ConfigMap exists" || pass=false

  local ca_crt
  ca_crt=$(kubectl get configmap inter-service-ca -n "$NAMESPACE" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || echo "")
  if [[ -n "$ca_crt" ]]; then
    tap_ok "ST-TLS-INTERSERVICE-004: inter-service-ca has ca.crt"
  else
    tap_not_ok "ST-TLS-INTERSERVICE-004: inter-service-ca has ca.crt" "ca.crt missing"
    pass=false
  fi

  # Verify leaf certs are signed by the CA
  local ca_pem
  ca_pem=$(kubectl get secret authwebhook-tls -n "$NAMESPACE" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
  if [[ -n "$ca_pem" ]]; then
    for secret_name in datastorage-tls gateway-tls kubernautagent-tls; do
      local leaf_pem
      leaf_pem=$(kubectl get secret "$secret_name" -n "$NAMESPACE" \
        -o jsonpath='{.data.tls\.crt}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
      if [[ -n "$leaf_pem" ]]; then
        if printf '%s' "$leaf_pem" | openssl verify -CAfile <(printf '%s' "$ca_pem") 2>/dev/null | grep -q "OK"; then
          tap_ok "ST-TLS-INTERSERVICE-005: ${secret_name} cert signed by shared CA"
        else
          tap_not_ok "ST-TLS-INTERSERVICE-005: ${secret_name} cert signed by shared CA" "verification failed"
          pass=false
        fi
      fi
    done
  fi

  # Verify ECDSA P-256 key type (Issue #753 2F)
  local ds_cert_pem
  ds_cert_pem=$(kubectl get secret datastorage-tls -n "$NAMESPACE" \
    -o jsonpath='{.data.tls\.crt}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
  if [[ -n "$ds_cert_pem" ]]; then
    local key_type
    key_type=$(printf '%s' "$ds_cert_pem" | openssl x509 -noout -text 2>/dev/null | grep "Public Key Algorithm" || echo "")
    if grep -qi "ec\|ecdsa" <<< "$key_type"; then
      tap_ok "ST-TLS-INTERSERVICE-006: Leaf certs use ECDSA (not RSA)"
    else
      tap_not_ok "ST-TLS-INTERSERVICE-006: Leaf certs use ECDSA (not RSA)" "key type: ${key_type}"
      pass=false
    fi
  fi
}

run_upg_001() {
  local desc_crd="ST-CHART-UPG-001a: CRD apply before upgrade"
  assert_exit_code "$desc_crd" kubectl apply --server-side --force-conflicts -f "${CHART_PATH}/crds/"

  local flags
  flags="$(common_install_flags) $(tls_flags)"

  # shellcheck disable=SC2046
  if helm upgrade kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" \
    $(production_secret_flags) \
    $flags \
    --timeout 5m >/dev/null 2>&1; then
    tap_ok "ST-CHART-UPG-001b: helm upgrade succeeds"
  else
    tap_not_ok "ST-CHART-UPG-001b: helm upgrade succeeds" "helm upgrade failed"
    return 1
  fi

  local revision
  revision=$(helm status kubernaut -n "$NAMESPACE" -o json 2>/dev/null | grep -o '"version": *[0-9]*' | grep -o '[0-9]*' || echo "0")
  if [[ "$revision" -ge 2 ]]; then
    tap_ok "ST-CHART-UPG-001c: Revision incremented to ${revision}"
  else
    tap_not_ok "ST-CHART-UPG-001c: Revision incremented" "Revision is ${revision}, expected >= 2"
  fi

  assert_pods_ready 13 "ST-CHART-UPG-001d: 13 pods healthy after upgrade"
}

# BR-PLATFORM-004 / DD-018: a second `helm install` on a cluster that already has a
# Kubernaut release must fail fast with the single-install guard's message, before any
# resources are applied — not with Helm's generic ownership-conflict error. Requires a
# live cluster (the guard's `lookup` is a no-op under `helm template`).
run_guard_001() {
  local desc="ST-CHART-GUARD-001: Second helm install on a different namespace is blocked by the single-install guard"
  local guard_ns="kubernaut-guard-test"
  local guard_release="kubernaut-guard-test"

  kubectl create namespace "$guard_ns" >/dev/null 2>&1 || true

  # Mirror run_pre_003: the guard must be reached (and be the *only* failure) before any
  # unrelated required-value check (e.g. missing secrets) short-circuits template rendering.
  local test_password="smoke-guard-test-pass"
  kubectl create secret generic kubernaut-pg-credentials \
    --from-literal=POSTGRES_USER=slm_user \
    --from-literal=POSTGRES_PASSWORD="$test_password" \
    --from-literal=POSTGRES_DB=action_history \
    --from-literal="db-secrets.yaml=$(printf 'username: slm_user\npassword: %s' "$test_password")" \
    -n "$guard_ns" >/dev/null 2>&1
  kubectl create secret generic kubernaut-valkey-credentials \
    --from-literal="valkey-secrets.yaml=password: \"${test_password}\"" \
    -n "$guard_ns" >/dev/null 2>&1
  kubectl create secret generic kubernaut-llm-credentials \
    --from-literal=OPENAI_API_KEY=sk-smoke-test-placeholder \
    -n "$guard_ns" >/dev/null 2>&1 # pre-commit:allow-sensitive

  local flags
  flags="$(common_install_flags) $(tls_flags)"

  local output
  # shellcheck disable=SC2046
  output=$(helm install "$guard_release" "$CHART_PATH" \
    --namespace "$guard_ns" \
    $(production_secret_flags) \
    $flags \
    --timeout 1m 2>&1)
  local exit_code=$?

  if [[ $exit_code -ne 0 ]] && grep -q "Kubernaut only supports one installation per cluster" <<< "$output"; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "expected guard failure message; exit=${exit_code} output=${output:0:300}"
  fi

  if ! helm list -n "$guard_ns" --short 2>/dev/null | grep -q "^${guard_release}$"; then
    tap_ok "ST-CHART-GUARD-001b: blocked install leaves no registered release (zero side effects)"
  else
    tap_not_ok "ST-CHART-GUARD-001b: blocked install leaves no registered release (zero side effects)" \
      "release was registered despite guard failure"
    helm uninstall "$guard_release" -n "$guard_ns" --no-hooks --timeout 1m >/dev/null 2>&1 || true
  fi

  kubectl delete namespace "$guard_ns" --ignore-not-found --timeout=60s >/dev/null 2>&1 || true
}

# BR-PLATFORM-006: the ST-CHART-CONSOLE-* template tests only prove the chart *renders*
# the console correctly — they use a fake, unreachable issuer ("issuer.example.com") and
# never observe a live Pod. That leaves the console's actual wiring point (does
# oauth2-proxy's OIDC discovery succeed and the container reach Ready?) with zero
# integration coverage, violating the pyramid invariant (IT proves wiring). This deploys
# a minimal, in-cluster Dex OIDC provider so run_console_live_001 can validate that
# wiring against a real (if lightweight) issuer instead of a placeholder URL.
#
# The `console` (nginx/SPA) container's image is built and published from a separate
# repository and is intentionally not available in this repo's CI — only the
# oauth2-proxy sidecar's readiness is asserted; the console container's ErrImagePull
# is expected and out of scope here.
DEX_ISSUER_URL=""
DEX_CLIENT_SECRET="smoke-test-dex-secret"

deploy_dex() {
  local desc="ST-CHART-CONSOLE-LIVE-000: Deploy Dex OIDC provider for live console validation"
  DEX_ISSUER_URL="http://dex.${NAMESPACE}.svc.cluster.local:5556/dex"
  # Pinned by digest (not tag) for reproducibility. This is the multi-arch manifest-list
  # digest for dexidp/dex:v2.45.1-alpine — container runtimes resolve the correct per-arch
  # (amd64/arm64/...) image from it automatically, so this works unchanged in CI (amd64)
  # and on Apple Silicon dev machines (arm64). To bump the version, resolve the new tag's
  # index digest with: skopeo inspect docker://docker.io/dexidp/dex:<new-tag> | jq -r .Digest
  local dex_image="docker.io/dexidp/dex@sha256:8499afd690c437f52301efd2b05b2455da5bd2dfc20332cd697dc9937f808462"

  # Preload into Kind via podman, mirroring the image-loading pattern already used
  # by CI (podman save | kind load image-archive) rather than `docker pull` (which
  # preload_hook_image uses but which is not guaranteed to exist on podman-only runners).
  if podman pull "$dex_image" >/dev/null 2>&1; then
    podman save "$dex_image" | kind load image-archive /dev/stdin --name "${KIND_CLUSTER_NAME:-kind}" >/dev/null 2>&1 || true
  fi

  kubectl apply -n "$NAMESPACE" -f - >/dev/null 2>&1 <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: dex-config
data:
  config.yaml: |
    issuer: ${DEX_ISSUER_URL}
    storage:
      type: memory
    web:
      http: 0.0.0.0:5556
    staticClients:
    - id: kubernaut-console
      secret: ${DEX_CLIENT_SECRET}
      redirectURIs:
      - 'https://console.smoke-test.local/oauth2/callback'
      name: 'Kubernaut Console'
    enablePasswordDB: true
    staticPasswords:
    - email: "tester@example.com"
      hash: "\$2a\$10\$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
      username: "tester"
      userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dex
  labels:
    app: dex
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dex
  template:
    metadata:
      labels:
        app: dex
    spec:
      containers:
      - name: dex
        image: ${dex_image}
        args: ["dex", "serve", "/etc/dex/config.yaml"]
        ports:
        - containerPort: 5556
        volumeMounts:
        - name: config
          mountPath: /etc/dex
        readinessProbe:
          httpGet:
            path: /dex/healthz
            port: 5556
          initialDelaySeconds: 2
          periodSeconds: 3
      volumes:
      - name: config
        configMap:
          name: dex-config
---
apiVersion: v1
kind: Service
metadata:
  name: dex
spec:
  selector:
    app: dex
  ports:
  - port: 5556
    targetPort: 5556
EOF

  if ! kubectl rollout status deployment/dex -n "$NAMESPACE" --timeout=60s >/dev/null 2>&1; then
    tap_not_ok "$desc" "Dex deployment did not become ready within 60s"
    return 1
  fi

  kubectl create secret generic console-oauth-creds -n "$NAMESPACE" \
    --from-literal=client-id=kubernaut-console \
    --from-literal=client-secret="$DEX_CLIENT_SECRET" \
    --from-literal=cookie-secret="$(openssl rand -hex 16)" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1

  tap_ok "$desc"
}

run_console_live_001() {
  local desc="ST-CHART-CONSOLE-LIVE-001: BR-PLATFORM-006 — console's oauth2-proxy reaches Ready via real OIDC discovery"

  if ! deploy_dex; then
    tap_not_ok "$desc" "prerequisite Dex deployment failed"
    return 1
  fi

  if ! helm upgrade kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" --reuse-values \
    --set console.enabled=true \
    --set console.auth.secretName=console-oauth-creds \
    --set console.ingress.enabled=false \
    --set console.ingress.host=console.smoke-test.local \
    --set apifrontend.config.auth.issuerURL="$DEX_ISSUER_URL" \
    --set apifrontend.config.auth.audience="$DEX_ISSUER_URL" \
    --timeout 2m >/dev/null 2>&1; then
    tap_not_ok "$desc" "helm upgrade to enable console failed"
    return 1
  fi

  local ready="" restarts="" attempt
  for attempt in $(seq 1 15); do
    ready=$(kubectl get pod -n "$NAMESPACE" -l app=console \
      -o jsonpath='{.items[0].status.containerStatuses[?(@.name=="oauth2-proxy")].ready}' 2>/dev/null || echo "")
    restarts=$(kubectl get pod -n "$NAMESPACE" -l app=console \
      -o jsonpath='{.items[0].status.containerStatuses[?(@.name=="oauth2-proxy")].restartCount}' 2>/dev/null || echo "0")
    [[ "$ready" == "true" ]] && break
    sleep 4
  done

  local logs
  logs=$(kubectl logs -n "$NAMESPACE" -l app=console -c oauth2-proxy --tail=50 2>/dev/null || echo "")

  if [[ "$ready" == "true" && "${restarts:-0}" -eq 0 ]] && \
     grep -q "OAuthProxy configured for OpenID Connect Client ID: kubernaut-console" <<< "$logs"; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "oauth2-proxy not Ready/stable via real OIDC discovery (ready=${ready:-unknown}, restarts=${restarts:-unknown})"
  fi
}

# BR-PLATFORM-003: the ST-CHART-HPA-* template tests only prove the chart *renders* a
# correct HorizontalPodAutoscaler manifest — they never observe a live object. Since
# autoscaling/v2 is a stable core API already present in every Kind node (unlike
# ServiceMonitor/PrometheusRule, which need the Prometheus Operator CRDs), this asserts
# the real, in-cluster HPA objects that a `helm upgrade --set *.autoscaling.enabled=true`
# produces, closing the wiring-verification gap called out in BR-PLATFORM-003's own
# non-goals section (live counterpart to ST-CHART-HPA-001/ST-CHART-MON-001/002).
run_mon_003() {
  local desc="ST-CHART-MON-003: BR-PLATFORM-003 — DataStorage/APIFrontend HorizontalPodAutoscaler is a real, correctly-configured autoscaling/v2 object"

  if ! helm upgrade kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" --reuse-values \
    --set datastorage.autoscaling.enabled=true \
    --set datastorage.autoscaling.minReplicas=1 \
    --set datastorage.autoscaling.maxReplicas=3 \
    --set apifrontend.autoscaling.enabled=true \
    --set apifrontend.autoscaling.minReplicas=1 \
    --set apifrontend.autoscaling.maxReplicas=4 \
    --timeout 2m >/dev/null 2>&1; then
    tap_not_ok "$desc" "helm upgrade to enable autoscaling failed"
    return 1
  fi

  local ds_min ds_max ds_target_kind ds_target_name
  ds_min=$(kubectl get hpa datastorage -n "$NAMESPACE" -o jsonpath='{.spec.minReplicas}' 2>/dev/null || echo "")
  ds_max=$(kubectl get hpa datastorage -n "$NAMESPACE" -o jsonpath='{.spec.maxReplicas}' 2>/dev/null || echo "")
  ds_target_kind=$(kubectl get hpa datastorage -n "$NAMESPACE" -o jsonpath='{.spec.scaleTargetRef.kind}' 2>/dev/null || echo "")
  ds_target_name=$(kubectl get hpa datastorage -n "$NAMESPACE" -o jsonpath='{.spec.scaleTargetRef.name}' 2>/dev/null || echo "")

  if [[ "$ds_min" == "1" && "$ds_max" == "3" && "$ds_target_kind" == "Deployment" && "$ds_target_name" == "datastorage" ]]; then
    tap_ok "ST-CHART-MON-003a: DataStorage HorizontalPodAutoscaler is live with configured min/maxReplicas + scaleTargetRef"
  else
    tap_not_ok "ST-CHART-MON-003a: DataStorage HPA live state" \
      "min=${ds_min:-unset} max=${ds_max:-unset} targetKind=${ds_target_kind:-unset} targetName=${ds_target_name:-unset}"
  fi

  local af_min af_max af_target_name
  af_min=$(kubectl get hpa apifrontend -n "$NAMESPACE" -o jsonpath='{.spec.minReplicas}' 2>/dev/null || echo "")
  af_max=$(kubectl get hpa apifrontend -n "$NAMESPACE" -o jsonpath='{.spec.maxReplicas}' 2>/dev/null || echo "")
  af_target_name=$(kubectl get hpa apifrontend -n "$NAMESPACE" -o jsonpath='{.spec.scaleTargetRef.name}' 2>/dev/null || echo "")

  if [[ "$af_min" == "1" && "$af_max" == "4" && "$af_target_name" == "apifrontend" ]]; then
    tap_ok "ST-CHART-MON-003b: APIFrontend HorizontalPodAutoscaler is live with configured min/maxReplicas"
  else
    tap_not_ok "ST-CHART-MON-003b: APIFrontend HPA live state" \
      "min=${af_min:-unset} max=${af_max:-unset} targetName=${af_target_name:-unset}"
  fi

  # HPA .status.currentMetrics requires metrics-server, which isn't guaranteed to be
  # installed in the smoke-test Kind cluster; asserting the live *spec* (real object +
  # values wiring) proves the wiring point without depending on metrics-server.

  # Revert so the release returns to its documented default (autoscaling disabled) for
  # the rest of the test run; uninstall would remove the HPAs regardless, but this keeps
  # the release's rendered state consistent with defaults for as long as it's still up.
  helm upgrade kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" --reuse-values \
    --set datastorage.autoscaling.enabled=false \
    --set apifrontend.autoscaling.enabled=false \
    --timeout 2m >/dev/null 2>&1 || true
}

run_uninst_001() {
  # The chart's pre-delete hook (webhook-cleanup Job) removes admission webhooks
  # before Helm deletes the release resources, preventing failurePolicy=Fail
  # rejections when the authwebhook pod terminates before CRs are cleaned up.
  #
  # Capture pre-uninstall state: if the pre-delete hook hangs (e.g. ImagePullBackOff
  # on bitnami/kubectl), the must-gather taken AFTER timeout won't show the hook pod.
  echo "# Pre-uninstall snapshot: jobs and hook pods"
  kubectl get jobs,pods -n "$NAMESPACE" --show-labels 2>/dev/null | sed 's/^/#   /' || true

  local uninstall_start uninstall_end uninstall_elapsed
  uninstall_start=$(date +%s)
  local uninstall_output
  uninstall_output=$(helm uninstall kubernaut -n "$NAMESPACE" --timeout 3m 2>&1)
  local uninstall_rc=$?
  uninstall_end=$(date +%s)
  uninstall_elapsed=$((uninstall_end - uninstall_start))

  if [[ $uninstall_rc -eq 0 ]]; then
    tap_ok "ST-CHART-UNINST-001a: helm uninstall succeeds (${uninstall_elapsed}s)"
    if [[ $uninstall_elapsed -gt 120 ]]; then
      echo "# WARNING: uninstall took ${uninstall_elapsed}s (>120s) — pre-delete hook may have been slow"
      must_gather "$NAMESPACE" "slow-uninstall"
    fi
  else
    tap_not_ok "ST-CHART-UNINST-001a: helm uninstall succeeds" "failed after ${uninstall_elapsed}s: ${uninstall_output}"
    must_gather "$NAMESPACE" "uninstall-failure"
    return 1
  fi

  sleep 10

  assert_resource_exists pvc postgresql-data "$NAMESPACE" \
    "ST-CHART-UNINST-001b: PostgreSQL PVC retained"

  assert_resource_exists pvc valkey-data "$NAMESPACE" \
    "ST-CHART-UNINST-001c: Valkey PVC retained"

  local crd_count
  crd_count=$(kubectl get crds 2>/dev/null | grep -c "kubernaut.ai" || true)
  if [[ "$crd_count" -eq 10 ]]; then
    tap_ok "ST-CHART-UNINST-001d: 10 CRDs retained"
  else
    tap_not_ok "ST-CHART-UNINST-001d: 10 CRDs retained" "Found ${crd_count} CRDs"
  fi
}

run_uninst_002() {
  local pass=true

  # Controllers are already gone (run_uninst_001 did helm uninstall).
  # Strip finalizers from any remaining CRs so CRD deletion doesn't hang.
  for crd in $(kubectl get crds -o name 2>/dev/null | grep kubernaut.ai); do
    local kind
    kind=$(kubectl get "$crd" -o jsonpath='{.spec.names.plural}' 2>/dev/null)
    if [[ -n "$kind" ]]; then
      kubectl get "$kind" --all-namespaces -o json 2>/dev/null \
        | python3 -c "
import json, sys
data = json.load(sys.stdin)
for item in data.get('items', []):
    if item.get('metadata', {}).get('finalizers'):
        ns = item['metadata'].get('namespace', '')
        name = item['metadata']['name']
        print(f'{ns}/{name}' if ns else name)
" 2>/dev/null | while read -r ref; do
          local cr_ns="${ref%%/*}"
          local cr_name="${ref##*/}"
          if [[ "$ref" == */* ]]; then
            kubectl patch "$kind" "$cr_name" -n "$cr_ns" --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
          else
            kubectl patch "$kind" "$cr_name" --type=merge -p '{"metadata":{"finalizers":null}}' 2>/dev/null || true
          fi
        done
    fi
  done

  kubectl delete pvc postgresql-data valkey-data -n "$NAMESPACE" --timeout=30s >/dev/null 2>&1 || pass=false
  kubectl delete -f "${CHART_PATH}/crds/" --timeout=60s >/dev/null 2>&1 || pass=false
  kubectl delete namespace "$NAMESPACE" --timeout=60s >/dev/null 2>&1 || pass=false

  sleep 10

  local crd_count
  crd_count=$(kubectl get crds 2>/dev/null | grep -c "kubernaut.ai" || true)
  if [[ "$crd_count" -eq 0 ]] && $pass; then
    tap_ok "ST-CHART-UNINST-002: Full cleanup complete"
  else
    tap_not_ok "ST-CHART-UNINST-002: Full cleanup complete" "CRDs remaining: ${crd_count}, pass: ${pass}"
  fi
}

preload_hook_image() {
  local desc="ST-CHART-PRELOAD: Pre-load Helm hook image into Kind cluster"
  local hook_image
  hook_image=$(grep -A5 'tlsCerts:' "$CHART_PATH/values.yaml" | grep 'image:' | awk '{print $2}' | head -1)

  if [[ -z "$hook_image" ]]; then
    tap_not_ok "$desc" "Could not determine hook image from chart values"
    return 0
  fi

  echo "# Pre-loading hook image: ${hook_image}"
  if docker pull "$hook_image" >/dev/null 2>&1 && \
     kind load docker-image "$hook_image" --name "$KIND_CLUSTER_NAME" >/dev/null 2>&1; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "Failed to pre-load ${hook_image} (Docker Hub rate limit?)"
  fi
}

run_verify_policies() {
  local pass=true

  # ST-CHART-VERIFY-POLICY-001: AI Analysis policy ConfigMap exists with approval.rego key
  local aia_key
  aia_key=$(kubectl get configmap aianalysis-policies -n "$NAMESPACE" \
    -o jsonpath='{.data.approval\.rego}' 2>/dev/null || echo "")
  if [[ -n "$aia_key" ]]; then
    tap_ok "ST-CHART-VERIFY-POLICY-001: aianalysis-policies ConfigMap exists with approval.rego"
  else
    tap_not_ok "ST-CHART-VERIFY-POLICY-001: aianalysis-policies ConfigMap" \
      "ConfigMap missing or approval.rego key empty"
    pass=false
  fi

  # ST-CHART-VERIFY-POLICY-002: Signal Processing policy ConfigMap exists with policy.rego key
  local sp_key
  sp_key=$(kubectl get configmap signalprocessing-policy -n "$NAMESPACE" \
    -o jsonpath='{.data.policy\.rego}' 2>/dev/null || echo "")
  if [[ -n "$sp_key" ]]; then
    tap_ok "ST-CHART-VERIFY-POLICY-002: signalprocessing-policy ConfigMap exists with policy.rego"
  else
    tap_not_ok "ST-CHART-VERIFY-POLICY-002: signalprocessing-policy ConfigMap" \
      "ConfigMap missing or policy.rego key empty"
    pass=false
  fi

  $pass
}

run_edge_001() {
  local desc="ST-CHART-EDGE-001: Stuck workflow namespace recovery"
  kubectl get all -n kubernaut-workflows >/dev/null 2>&1 || true
  kubectl delete jobs --all -n kubernaut-workflows >/dev/null 2>&1 || true

  local phase
  phase=$(kubectl get ns kubernaut-workflows -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
  if [[ "$phase" == "Active" || "$phase" == "NotFound" ]]; then
    tap_ok "$desc (namespace phase: ${phase})"
  else
    tap_not_ok "$desc" "Namespace stuck in phase: ${phase}"
  fi
}

# ---------------------------------------------------------------------------
# Flow orchestration
# ---------------------------------------------------------------------------

flow_a_production() {
  echo "# --- Flow A: Production Install Lifecycle (${PLATFORM}) ---"
  local flow_failed=false

  run_pre_001
  run_pre_002
  run_pre_003
  run_pre_004
  run_inst_001 || { echo "# FAIL-FAST: helm install failed, skipping remaining Flow A tests"; must_gather "$NAMESPACE" "install-failure"; return 1; }

  if [[ "$PLATFORM" == "kind" ]]; then
    run_tls_patch
  fi

  run_verify_001 || flow_failed=true
  run_verify_002 || flow_failed=true
  run_verify_003 || flow_failed=true
  run_verify_policies || flow_failed=true

  run_tls_001
  run_tls_interservice

  run_upg_001 || flow_failed=true

  run_edge_001
  run_guard_001
  run_mon_003 || flow_failed=true

  if [[ "$PLATFORM" == "kind" ]]; then
    run_console_live_001 || flow_failed=true
  fi

  if $flow_failed; then
    must_gather "$NAMESPACE" "flow-a-verification-failure"
  fi

  run_uninst_001
  run_uninst_002
}

flow_b_quickstart() {
  echo "# --- Flow B: Dev Quick Start Lifecycle (kind only) ---"
  local flow_failed=false

  kubectl create namespace "$NAMESPACE" >/dev/null 2>&1 || true
  run_pre_003
  run_pre_004
  run_inst_003 || { echo "# FAIL-FAST: helm install failed, skipping remaining Flow B tests"; must_gather "$NAMESPACE" "install-failure"; return 1; }
  run_tls_patch
  run_verify_001 || flow_failed=true
  run_verify_policies || flow_failed=true
  run_tls_interservice
  run_edge_001

  if $flow_failed; then
    must_gather "$NAMESPACE" "flow-b-verification-failure"
  fi

  run_uninst_001
  run_uninst_002
}

# ---------------------------------------------------------------------------
# Flow C: cert-manager TLS mode lifecycle
#
# #334 + DD-PLATFORM-001 regression guard. Before this flow existed, CI only
# ever exercised tls.mode=hook (see tls_flags() above) -- the datastorage-
# signing-cert gap (#334) and the inter-service mTLS gap (DD-PLATFORM-001)
# were BOTH silent in cert-manager mode and were only caught by manual kind
# spikes, never by CI. This flow makes tls.mode=cert-manager a first-class,
# continuously-verified deployment path.
#
# Prerequisite (provided by the CI workflow, not this script): cert-manager
# installed and exactly one ClusterIssuer present in the cluster. The chart
# auto-selects it via `lookup` on a real `helm install` (see
# tls.certManager.issuerRef in values.yaml), so no --set is needed here.
# ---------------------------------------------------------------------------
run_inst_cm_001() {
  local desc="ST-CHART-INST-CM-001: Production install (tls.mode=cert-manager)"
  local flags
  flags="$(common_install_flags) --set tls.mode=cert-manager"

  # shellcheck disable=SC2046
  if helm install kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" \
    $(production_secret_flags) \
    $flags \
    --timeout 5m >/dev/null; then
    tap_ok "$desc"
  else
    tap_not_ok "$desc" "helm install failed"
    return 1
  fi
}

# #334 regression guard: DataStorage's AU-9 audit-signing key must be
# auto-provisioned in cert-manager mode, not just hook mode.
run_tls_cm_datastorage() {
  local pass=true
  assert_resource_exists secret datastorage-signing-cert "$NAMESPACE" \
    "ST-CHART-TLS-CM-001a: datastorage-signing-cert Secret exists (#334)" || pass=false

  local key
  key=$(kubectl get secret datastorage-signing-cert -n "$NAMESPACE" \
    -o jsonpath='{.data.tls\.key}' 2>/dev/null || echo "")
  if [[ -n "$key" ]]; then
    tap_ok "ST-CHART-TLS-CM-001b: datastorage-signing-cert Secret contains tls.key"
  else
    tap_not_ok "ST-CHART-TLS-CM-001b: datastorage-signing-cert Secret contains tls.key" "tls.key missing"
    pass=false
  fi

  $pass
}

# DD-PLATFORM-001 regression guard: the dedicated internal CA, its three leaf
# certs, and the inter-service-ca ConfigMap sync hook must all provision
# correctly and be internally consistent (not just individually present).
run_tls_cm_interservice() {
  local pass=true

  assert_resource_exists secret kubernaut-interservice-ca-secret "$NAMESPACE" \
    "ST-CHART-TLS-CM-002a: kubernaut-interservice-ca-secret exists (DD-PLATFORM-001)" || pass=false

  local leaf
  for leaf in gateway-tls datastorage-tls kubernautagent-tls; do
    assert_resource_exists secret "$leaf" "$NAMESPACE" \
      "ST-CHART-TLS-CM-002b: ${leaf} leaf Secret exists" || pass=false
  done

  assert_resource_exists configmap inter-service-ca "$NAMESPACE" \
    "ST-CHART-TLS-CM-002c: inter-service-ca ConfigMap exists (synced by hook)" || pass=false

  # Cross-check: the ConfigMap's ca.crt must match the CA secret's ca.crt --
  # proves the sync hook copied the right bytes, not stale/empty data.
  local ca_from_secret ca_from_cm
  ca_from_secret=$(kubectl get secret kubernaut-interservice-ca-secret -n "$NAMESPACE" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
  ca_from_cm=$(kubectl get configmap inter-service-ca -n "$NAMESPACE" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || echo "")
  if [[ -n "$ca_from_secret" && "$ca_from_secret" == "$ca_from_cm" ]]; then
    tap_ok "ST-CHART-TLS-CM-002d: inter-service-ca ConfigMap matches CA secret"
  else
    tap_not_ok "ST-CHART-TLS-CM-002d: inter-service-ca ConfigMap matches CA secret" "mismatch or empty"
    pass=false
  fi

  # gateway-tls must chain to the interservice CA specifically (not some other
  # CA, e.g. authwebhook's) -- verifies issuerRef wiring end-to-end, not just
  # "a cert exists".
  local tmpdir
  tmpdir=$(mktemp -d)
  kubectl get secret kubernaut-interservice-ca-secret -n "$NAMESPACE" \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d > "$tmpdir/ca.crt" 2>/dev/null
  kubectl get secret gateway-tls -n "$NAMESPACE" \
    -o jsonpath='{.data.tls\.crt}' 2>/dev/null | base64 -d > "$tmpdir/gateway.crt" 2>/dev/null
  if openssl verify -CAfile "$tmpdir/ca.crt" "$tmpdir/gateway.crt" >/dev/null 2>&1; then
    tap_ok "ST-CHART-TLS-CM-002e: gateway-tls chains to the interservice CA"
  else
    tap_not_ok "ST-CHART-TLS-CM-002e: gateway-tls chains to the interservice CA" \
      "openssl verify failed -- leaf not signed by kubernaut-interservice-ca-secret"
    pass=false
  fi
  rm -rf "$tmpdir"

  $pass
}

flow_c_cert_manager() {
  echo "# --- Flow C: cert-manager TLS Mode Lifecycle (kind only) ---"
  local flow_failed=false

  run_pre_001
  run_pre_002
  run_pre_003
  # NOTE: run_pre_004 (manual authwebhook-tls pre-seed) is intentionally
  # skipped -- in cert-manager mode, cert-manager owns that Secret's
  # lifecycle via the authwebhook-cert Certificate resource.
  run_inst_cm_001 || { echo "# FAIL-FAST: helm install failed, skipping remaining Flow C tests"; must_gather "$NAMESPACE" "install-failure"; return 1; }

  run_verify_001 || flow_failed=true
  run_verify_002 || flow_failed=true
  run_verify_003 || flow_failed=true
  run_verify_policies || flow_failed=true

  run_tls_001 || flow_failed=true
  run_tls_cm_datastorage || flow_failed=true
  run_tls_cm_interservice || flow_failed=true

  if $flow_failed; then
    must_gather "$NAMESPACE" "flow-c-verification-failure"
  fi

  run_uninst_001
  run_uninst_002
}

# ---------------------------------------------------------------------------
# Template tests (no cluster required)
# Issue #390: Validate ConfigMap split, prometheus, and SDK config tiers
# ---------------------------------------------------------------------------
template_common_args() {
  echo "--set" "postgresql.auth.existingSecret=dummy"
  echo "--set" "valkey.existingSecret=dummy"
}

template_llm_args() {
  echo "--set" "kubernautAgent.llm.provider=openai"
  echo "--set" "kubernautAgent.llm.model=gpt-4"
}

run_template_tests() {
  echo "# --- Template Tests: Issue #390 ConfigMap Split ---"
  local tpl_flag="-s"
  local tpl_path="templates/kubernaut-agent/kubernaut-agent.yaml"
  local output

  # IT-HAPI-390-001: Single ConfigMap rendered (SDK ConfigMap removed — consolidated)
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) 2>&1)
  if grep -q "name: kubernaut-agent-config" <<< "$output" && \
     ! grep -q "name: kubernaut-agent-sdk-config" <<< "$output"; then
    tap_ok "IT-HAPI-390-001: helm template renders kubernaut-agent-config only (SDK ConfigMap removed)"
  else
    tap_not_ok "IT-HAPI-390-001: consolidated ConfigMap" "SDK ConfigMap still present or main ConfigMap missing"
  fi

  # IT-HAPI-390-002: LLM static config in main, runtime in separate ConfigMap
  if grep -q "provider:" <<< "$output" && \
     grep -q "kubernaut-agent-llm-runtime" <<< "$output"; then
    tap_ok "IT-HAPI-390-002: LLM provider in main ConfigMap, runtime in separate ConfigMap"
  else
    tap_not_ok "IT-HAPI-390-002: LLM config split" "provider not in main or runtime ConfigMap missing"
  fi

  # IT-HAPI-390-003: No SDK volume mount in Deployment
  if ! grep -q "mountPath: /etc/kubernaut-agent/sdk" <<< "$output"; then
    tap_ok "IT-HAPI-390-003: Deployment has no SDK volume mount (consolidated)"
  else
    tap_not_ok "IT-HAPI-390-003: SDK volume mount removal" "SDK mount still present"
  fi

  # IT-HAPI-390-004: helm lint passes
  if helm lint "$CHART_PATH" $(template_common_args) $(template_llm_args) $(policy_flags) >/dev/null 2>&1; then
    tap_ok "IT-HAPI-390-004: helm lint passes for consolidated config"
  else
    tap_not_ok "IT-HAPI-390-004: helm lint" "lint failed"
  fi

  echo "# --- Template Tests: Helm Hook Hardening ---"

  # ST-HOOK-TPL-001: webhook count parsing uses jsonpath (not fragile grep)
  local hook_tpl
  hook_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    -s templates/hooks/tls-cert-job.yaml 2>&1)
  local webhook_tpl
  webhook_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    -s templates/authwebhook/authwebhook.yaml 2>&1)

  if grep -q "jsonpath='{.webhooks\[\\*\].name}'" <<< "$hook_tpl" && \
     grep -q "jsonpath='{.webhooks\[\\*\].name}'" <<< "$webhook_tpl"; then
    tap_ok "ST-HOOK-TPL-001: webhook count parsing uses jsonpath (not grep)"
  else
    tap_not_ok "ST-HOOK-TPL-001: webhook count parsing uses jsonpath" \
      "One or more templates still use grep-based webhook counting"
  fi

  # ST-HOOK-TPL-002: no hardcoded runAsUser/runAsGroup in hook jobs
  if ! grep -qE "runAsUser: 65534|runAsGroup: 65534" <<< "$hook_tpl" && \
     ! grep -qE "runAsUser: 65534|runAsGroup: 65534" <<< "$webhook_tpl"; then
    tap_ok "ST-HOOK-TPL-002: no hardcoded UID 65534 in hooks or authwebhook"
  else
    tap_not_ok "ST-HOOK-TPL-002: no hardcoded UID 65534" \
      "Found runAsUser/runAsGroup: 65534 in rendered templates"
  fi

  # ST-WEBHOOK-OPS-001: RemediationWorkflow webhook includes CREATE, UPDATE, DELETE (#773)
  local webhooks_tpl
  webhooks_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    -s templates/authwebhook/webhooks.yaml 2>&1)
  if grep -B5 "remediationworkflows" <<< "$webhooks_tpl" | grep -q "UPDATE"; then
    tap_ok "ST-WEBHOOK-OPS-001: RemediationWorkflow webhook includes UPDATE operation (#773)"
  else
    tap_not_ok "ST-WEBHOOK-OPS-001: RemediationWorkflow webhook includes UPDATE operation" \
      "UPDATE operation missing from remediationworkflows webhook operations"
  fi

  echo "# --- Template Tests: Rego Policy Mandatory Validation ---"

  local aia_tpl sp_tpl

  # ST-POLICY-001: Template fails when no policies are provided
  aia_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) 2>&1)
  if [[ $? -ne 0 ]] && grep -qE "is required" <<< "$aia_tpl"; then
    tap_ok "ST-POLICY-001: Template fails when no Rego policies are provided"
  else
    tap_not_ok "ST-POLICY-001: mandatory policy validation" \
      "Template should fail when no policies are provided"
  fi

  # ST-POLICY-002: Template fails with AA policy but without SP policy
  aia_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) \
    --set-file "aianalysis.policies.content=$POLICY_AA_FILE" 2>&1)
  if [[ $? -ne 0 ]] && grep -q "signalprocessing.policies.content is required" <<< "$aia_tpl"; then
    tap_ok "ST-POLICY-002: Template fails when SP policy is missing (AA provided)"
  else
    tap_not_ok "ST-POLICY-002: SP mandatory policy validation" \
      "Template should fail with SP required message when only AA policy is provided"
  fi

  # ST-POLICY-003: Template fails with SP policy but without AA policy
  aia_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) \
    --set-file "signalprocessing.policies.content=$POLICY_SP_FILE" 2>&1)
  if [[ $? -ne 0 ]] && grep -q "aianalysis.policies.content is required" <<< "$aia_tpl"; then
    tap_ok "ST-POLICY-003: Template fails when AA policy is missing (SP provided)"
  else
    tap_not_ok "ST-POLICY-003: AA mandatory policy validation" \
      "Template should fail with AA required message when only SP policy is provided"
  fi

  # ST-POLICY-004: Both --set-file renders AI Analysis approval policy ConfigMap
  aia_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) \
    $(policy_flags) \
    -s templates/aianalysis/aianalysis.yaml 2>&1)
  if grep -q "name: aianalysis-policies" <<< "$aia_tpl" && \
     grep -q "approval.rego" <<< "$aia_tpl"; then
    tap_ok "ST-POLICY-004: --set-file renders aianalysis-policies ConfigMap with approval.rego"
  else
    tap_not_ok "ST-POLICY-004: --set-file AA policy render" \
      "aianalysis-policies ConfigMap or approval.rego key missing"
  fi

  # ST-POLICY-005: Both --set-file renders Signal Processing policy ConfigMap
  sp_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) \
    $(policy_flags) \
    -s templates/signalprocessing/signalprocessing.yaml 2>&1)
  if grep -q "name: signalprocessing-policy" <<< "$sp_tpl" && \
     grep -q "policy.rego" <<< "$sp_tpl"; then
    tap_ok "ST-POLICY-005: --set-file renders signalprocessing-policy ConfigMap with policy.rego"
  else
    tap_not_ok "ST-POLICY-005: --set-file SP policy render" \
      "signalprocessing-policy ConfigMap or policy.rego key missing"
  fi

  # ST-POLICY-006: existingConfigMap skips chart-generated AI Analysis policy ConfigMap
  aia_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) \
    --set-file "signalprocessing.policies.content=$POLICY_SP_FILE" \
    --set "aianalysis.policies.existingConfigMap=my-custom-policies" \
    -s templates/aianalysis/aianalysis.yaml 2>&1)
  if ! grep -q "name: aianalysis-policies" <<< "$aia_tpl"; then
    tap_ok "ST-POLICY-006: existingConfigMap skips chart-generated aianalysis-policies ConfigMap"
  else
    tap_not_ok "ST-POLICY-006: existingConfigMap skip" \
      "aianalysis-policies ConfigMap still rendered when existingConfigMap is set"
  fi

  # ST-POLICY-007: policies.existingConfigMap skips chart-generated SP policy ConfigMap
  sp_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) \
    --set-file "aianalysis.policies.content=$POLICY_AA_FILE" \
    --set "signalprocessing.policies.existingConfigMap=my-sp-policy" \
    -s templates/signalprocessing/signalprocessing.yaml 2>&1)
  if ! grep -q "name: signalprocessing-policy" <<< "$sp_tpl"; then
    tap_ok "ST-POLICY-007: policies.existingConfigMap skips chart-generated signalprocessing-policy ConfigMap"
  else
    tap_not_ok "ST-POLICY-007: policies.existingConfigMap skip" \
      "signalprocessing-policy ConfigMap still rendered when policies.existingConfigMap is set"
  fi

  echo "# --- Template Tests: NetworkPolicy (Issue #285) ---"

  # ST-NP-001: Default renders 13 NetworkPolicies (enabled by default)
  # Count: 12 after removing orphaned legacy HAPI NP in v1.4, +1 for APIFrontend
  # (BR-PLATFORM-005, Issue #1589 follow-up).
  output=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set networkPolicies.apiServerCIDR=10.96.0.1/32 2>&1)
  local np_count
  np_count=$(grep -c "kind: NetworkPolicy" <<< "$output" || true)
  if [[ "$np_count" -eq 13 ]]; then
    tap_ok "ST-NP-001: default renders 13 NetworkPolicies (enabled by default)"
  else
    tap_not_ok "ST-NP-001: default should render 13 NetworkPolicies" \
      "Found ${np_count}"
  fi

  # ST-NP-002: Disabling renders zero policies
  output=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set networkPolicies.enabled=false 2>&1)
  np_count=$(grep -c "kind: NetworkPolicy" <<< "$output" || true)
  if [[ "$np_count" -eq 0 ]]; then
    tap_ok "ST-NP-002: networkPolicies.enabled=false renders zero NetworkPolicies"
  else
    tap_not_ok "ST-NP-002: expected zero NetworkPolicies when disabled" \
      "Found ${np_count}"
  fi

  # ST-NP-003: Every policy includes DNS egress (port 53)
  local np_without_dns=0
  while IFS= read -r policy_name; do
    local policy_yaml
    policy_yaml=$(echo "$output" | python3 -c "
import sys, yaml
docs = list(yaml.safe_load_all(sys.stdin))
for d in docs:
    if d and d.get('kind') == 'NetworkPolicy' and d.get('metadata',{}).get('name') == '$policy_name':
        egress = d.get('spec',{}).get('egress',[])
        has_dns = any(
            any(p.get('port') == 53 for p in r.get('ports',[]))
            for r in egress
        )
        if not has_dns:
            print('MISSING')
        break
" 2>/dev/null <<< "$output")
    if [[ "$policy_yaml" == "MISSING" ]]; then
      np_without_dns=$((np_without_dns + 1))
    fi
  done < <(grep -A1 "kind: NetworkPolicy" <<< "$output" | grep "name:" | awk '{print $2}')
  if [[ "$np_without_dns" -eq 0 ]]; then
    tap_ok "ST-NP-003: all 13 NetworkPolicies include DNS egress (port 53)"
  else
    tap_not_ok "ST-NP-003: DNS egress in all policies" \
      "${np_without_dns} policies missing DNS egress"
  fi

  # ST-NP-004: Per-service disable skips that policy
  output=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set networkPolicies.enabled=true \
    --set networkPolicies.apiServerCIDR=10.96.0.1/32 \
    --set networkPolicies.notification.enabled=false 2>&1)
  local np_notif_count
  np_notif_count=$(grep -A1 "kind: NetworkPolicy" <<< "$output" | grep -c "notification" || true)
  if [[ "$np_notif_count" -eq 0 ]]; then
    tap_ok "ST-NP-004: notification.enabled=false skips Notification NetworkPolicy"
  else
    tap_not_ok "ST-NP-004: per-service disable" \
      "Notification NetworkPolicy still rendered when disabled"
  fi

  # ST-NP-005: PostgreSQL/Valkey conditional on their enabled flags (F-7)
  # postgresql.host is required when postgresql.enabled=false (migration-job validation).
  # Count: 11 = 13 total - PG - VK (13 total after adding APIFrontend NetworkPolicy).
  output=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set networkPolicies.enabled=true \
    --set networkPolicies.apiServerCIDR=10.96.0.1/32 \
    --set postgresql.enabled=false \
    --set postgresql.host=external-pg.example.com \
    --set valkey.enabled=false \
    --set valkey.host=external-valkey.example.com 2>&1)
  np_count=$(grep -c "kind: NetworkPolicy" <<< "$output" || true)
  if [[ "$np_count" -eq 11 ]]; then
    tap_ok "ST-NP-005: postgresql/valkey disabled = 11 NetworkPolicies (no PG/VK)"
  else
    tap_not_ok "ST-NP-005: infra conditional rendering" \
      "Expected 11 policies without PG/VK, got ${np_count}"
  fi

  # ST-NP-006: helm lint passes with NetworkPolicies enabled
  if helm lint "$CHART_PATH" $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set networkPolicies.enabled=true \
    --set networkPolicies.apiServerCIDR=10.96.0.1/32 >/dev/null 2>&1; then
    tap_ok "ST-NP-006: helm lint passes with networkPolicies.enabled=true"
  else
    tap_not_ok "ST-NP-006: helm lint with NetworkPolicies" \
      "helm lint failed"
  fi

  echo "# --- Template Tests: GCP Conditional Fields ---"

  # GCP fields conditional: not rendered for non-vertex providers
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) 2>&1)
  if ! grep -q "gcp_project_id" <<< "$output"; then
    tap_ok "ST-SDK-GCP-001: gcp_project_id not rendered for non-vertex provider"
  else
    tap_not_ok "ST-SDK-GCP-001: gcp_project_id conditional" "gcp_project_id rendered for openai provider"
  fi

  # Note: Vertex AI / GCP-specific fields (gcpProjectId, gcpRegion) are configured
  # via the main config.yaml, not the quickstart provider/model values.

  echo "# --- Template Tests: LLM Reasoning/Thinking Config (BR-AI-086) ---"

  # ST-CHART-LLM-REASON-001a: default (reasoning unset) renders no reasoning block
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) 2>&1)
  if ! grep -q "reasoning:" <<< "$output"; then
    tap_ok "ST-CHART-LLM-REASON-001a: reasoning block absent by default"
  else
    tap_not_ok "ST-CHART-LLM-REASON-001a: reasoning block absent by default" \
      "reasoning: rendered despite reasoning.enabled=false and no capabilityOverride"
  fi

  # ST-CHART-LLM-REASON-001b: reasoning.enabled=true + budgetTokens renders both fields
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.llm.reasoning.enabled=true \
    --set kubernautAgent.llm.reasoning.budgetTokens=4096 2>&1)
  if grep -q "reasoning:" <<< "$output" && \
     grep -A2 "reasoning:" <<< "$output" | grep -q "enabled: true" && \
     grep -A2 "reasoning:" <<< "$output" | grep -q "budgetTokens: 4096"; then
    tap_ok "ST-CHART-LLM-REASON-001b: reasoning.enabled+budgetTokens render together"
  else
    tap_not_ok "ST-CHART-LLM-REASON-001b: reasoning.enabled+budgetTokens" \
      "reasoning block missing enabled:true or budgetTokens:4096"
  fi

  # ST-CHART-LLM-REASON-001c: capabilityOverride alone (enabled left false) still renders
  # (BR-AI-086 AC5: openai_compatible self-hosted models can force detection off/on
  # independently of the enabled toggle)
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(policy_flags) \
    --set kubernautAgent.llm.provider=openai_compatible \
    --set kubernautAgent.llm.model=custom-model \
    --set kubernautAgent.llm.reasoning.capabilityOverride=force_off 2>&1)
  if grep -A2 "reasoning:" <<< "$output" | grep -q 'capabilityOverride: "force_off"' && \
     ! grep -A2 "reasoning:" <<< "$output" | grep -q "enabled:"; then
    tap_ok "ST-CHART-LLM-REASON-001c: capabilityOverride renders without enabled"
  else
    tap_not_ok "ST-CHART-LLM-REASON-001c: capabilityOverride without enabled" \
      "capabilityOverride not rendered, or enabled leaked in alongside it"
  fi

  # ST-CHART-LLM-REASON-002: values.schema.json rejects an invalid capabilityOverride
  if ! helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.llm.reasoning.capabilityOverride=bogus >/dev/null 2>&1; then
    tap_ok "ST-CHART-LLM-REASON-002: schema rejects invalid reasoning.capabilityOverride"
  else
    tap_not_ok "ST-CHART-LLM-REASON-002: schema validation for reasoning.capabilityOverride" \
      "helm template succeeded with capabilityOverride=bogus (expected schema enum rejection)"
  fi

  # ST-CHART-LLM-REASON-003a: reasoning.effort alone (enabled left false) still renders
  # (#1604: effort is independently useful on OpenAI/DeepSeek models, which have no
  # separate "enabled" concept the way Anthropic's thinking param does)
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.llm.reasoning.effort=high 2>&1)
  if grep -A3 "reasoning:" <<< "$output" | grep -q 'effort: "high"' && \
     ! grep -A3 "reasoning:" <<< "$output" | grep -q "enabled:"; then
    tap_ok "ST-CHART-LLM-REASON-003a: effort renders without enabled"
  else
    tap_not_ok "ST-CHART-LLM-REASON-003a: effort without enabled" \
      "effort not rendered, or enabled leaked in alongside it"
  fi

  # ST-CHART-LLM-REASON-003b: reasoning.enabled=true + effort renders both fields
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.llm.reasoning.enabled=true \
    --set kubernautAgent.llm.reasoning.effort=medium 2>&1)
  if grep -q "reasoning:" <<< "$output" && \
     grep -A3 "reasoning:" <<< "$output" | grep -q "enabled: true" && \
     grep -A3 "reasoning:" <<< "$output" | grep -q 'effort: "medium"'; then
    tap_ok "ST-CHART-LLM-REASON-003b: reasoning.enabled+effort render together"
  else
    tap_not_ok "ST-CHART-LLM-REASON-003b: reasoning.enabled+effort" \
      "reasoning block missing enabled:true or effort:medium"
  fi

  # ST-CHART-LLM-REASON-004: values.schema.json rejects an invalid effort value
  if ! helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.llm.reasoning.effort=extreme >/dev/null 2>&1; then
    tap_ok "ST-CHART-LLM-REASON-004: schema rejects invalid reasoning.effort"
  else
    tap_not_ok "ST-CHART-LLM-REASON-004: schema validation for reasoning.effort" \
      "helm template succeeded with effort=extreme (expected schema enum rejection)"
  fi

  echo "# --- Template Tests: Unified Monitoring Config (Issue #463) ---"

  # UT-MON-463-001: monitoring.prometheus.enabled+url configures both EM and KA
  local mon_output
  mon_output=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set monitoring.prometheus.enabled=true \
    --set monitoring.prometheus.url=http://prom:9090 2>&1)
  if grep -q 'prometheusUrl: "http://prom:9090"' <<< "$mon_output" && \
     grep -q 'prometheusEnabled: true' <<< "$mon_output" && \
     grep -A3 "tools:" <<< "$mon_output" | grep -q 'url: "http://prom:9090"'; then
    tap_ok "UT-MON-463-001: monitoring.prometheus.enabled+url configures both EM and KA"
  else
    tap_not_ok "UT-MON-463-001: monitoring.prometheus.enabled+url configures both EM and KA" \
      "EM or KA ConfigMap missing monitoring.prometheus.url"
  fi

  # UT-MON-463-002: EM ConfigMap reads monitoring.prometheus.url
  if grep -q 'prometheusUrl: "http://prom:9090"' <<< "$mon_output"; then
    tap_ok "UT-MON-463-002: EM ConfigMap prometheusUrl from monitoring.prometheus.url"
  else
    tap_not_ok "UT-MON-463-002: EM ConfigMap prometheusUrl" \
      "EM ConfigMap does not contain prometheusUrl from monitoring.prometheus.url"
  fi

  # UT-MON-463-003: KA config.yaml has tools.prometheus.url (not SDK toolsets)
  if grep -A3 "tools:" <<< "$mon_output" | grep -q 'url: "http://prom:9090"'; then
    tap_ok "UT-MON-463-003: KA config.yaml tools.prometheus.url from monitoring"
  else
    tap_not_ok "UT-MON-463-003: KA config.yaml tools.prometheus.url" \
      "KA config.yaml does not contain tools.prometheus.url"
  fi

  # UT-MON-463-004: monitoring.prometheus.enabled=false disables in both
  local mon_off
  mon_off=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set monitoring.prometheus.enabled=false 2>&1)
  if grep -q 'prometheusEnabled: false' <<< "$mon_off" && \
     ! grep -A3 "tools:" <<< "$mon_off" | grep -q 'url:'; then
    tap_ok "UT-MON-463-004: monitoring.prometheus.enabled=false disables both EM and KA"
  else
    tap_not_ok "UT-MON-463-004: monitoring disabled" \
      "Prometheus not fully disabled when monitoring.prometheus.enabled=false"
  fi

  # UT-MON-463-005: both Prometheus and AlertManager enabled
  local mon_both
  mon_both=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set monitoring.prometheus.enabled=true \
    --set monitoring.prometheus.url=http://prom:9090 \
    --set monitoring.alertManager.enabled=true \
    --set monitoring.alertManager.url=http://am:9093 2>&1)
  if grep -q 'prometheusUrl: "http://prom:9090"' <<< "$mon_both" && \
     grep -q 'alertManagerUrl: "http://am:9093"' <<< "$mon_both" && \
     grep -q 'alertManagerEnabled: true' <<< "$mon_both"; then
    tap_ok "UT-MON-463-005: both Prometheus and AlertManager enabled"
  else
    tap_not_ok "UT-MON-463-005: both monitoring endpoints" \
      "Missing one or both monitoring endpoints in ConfigMaps"
  fi

  # UT-MON-463-009: TLS CA volume mounted on both EM and KA when tlsCaFile set
  local mon_tls
  mon_tls=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set monitoring.prometheus.enabled=true \
    --set monitoring.prometheus.url=https://prom:9091 \
    --set monitoring.prometheus.tlsCaFile=/etc/ssl/certs/service-ca.crt 2>&1)
  if grep -q "service-ca" <<< "$mon_tls" && \
     grep -q "/etc/ssl" <<< "$mon_tls"; then
    tap_ok "UT-MON-463-009: TLS CA volume mounted when tlsCaFile set"
  else
    tap_not_ok "UT-MON-463-009: TLS CA volume" \
      "service-ca volume or mount not found when tlsCaFile is set"
  fi

  # UT-MON-463-013: helm lint passes with monitoring block
  if helm lint "$CHART_PATH" $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set monitoring.prometheus.enabled=true \
    --set monitoring.prometheus.url=http://prom:9090 >/dev/null 2>&1; then
    tap_ok "UT-MON-463-013: helm lint passes with monitoring block"
  else
    tap_not_ok "UT-MON-463-013: helm lint with monitoring" \
      "helm lint failed with monitoring values"
  fi

  echo "# --- Template Tests: MCP Interactive Mode (#703 PR6a) ---"

  local interactive_out

  # HELM-01: interactive.enabled=true emits interactive: block in ConfigMap
  interactive_out=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.interactive.enabled=true 2>&1)
  if echo "$interactive_out" | grep -q "interactive:" && \
     echo "$interactive_out" | grep -q "enabled: true"; then
    tap_ok "HELM-01: interactive.enabled=true emits interactive: block in ConfigMap"
  else
    tap_not_ok "HELM-01: interactive ConfigMap block" \
      "interactive: block not found when interactive.enabled=true"
  fi

  # HELM-02: interactive.enabled=true adds Lease RBAC under namespace-scoped Role
  if echo "$interactive_out" | grep -q "coordination.k8s.io" && \
     echo "$interactive_out" | grep -q "leases" && \
     echo "$interactive_out" | grep -q "kind: Role" && \
     echo "$interactive_out" | grep -q "kubernaut-agent-interactive-leases"; then
    tap_ok "HELM-02: interactive.enabled=true adds coordination.k8s.io/leases under namespace-scoped Role"
  else
    tap_not_ok "HELM-02: Lease RBAC" \
      "coordination.k8s.io/leases RBAC not found under namespace-scoped Role when interactive enabled"
  fi

  # HELM-03: interactive.enabled=true does NOT include impersonate RBAC (#1288)
  if ! echo "$interactive_out" | grep -q "impersonate"; then
    tap_ok "HELM-03: interactive.enabled=true omits impersonate verb RBAC (#1288)"
  else
    tap_not_ok "HELM-03: impersonate RBAC still present" \
      "impersonate verb found when interactive enabled — should have been removed by #1288"
  fi

  # HELM-04: interactive.enabled=false omits interactive: block
  local interactive_off
  interactive_off=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) 2>&1)
  if ! echo "$interactive_off" | grep -q "interactive:"; then
    tap_ok "HELM-04: interactive disabled omits interactive: block"
  else
    tap_not_ok "HELM-04: interactive disabled should omit" \
      "interactive: found when interactive.enabled is false/unset"
  fi

  # HELM-05: custom sessionTTL and maxConcurrentSessions rendered
  interactive_out=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.interactive.enabled=true \
    --set kubernautAgent.interactive.sessionTTL=15m \
    --set kubernautAgent.interactive.maxConcurrentSessions=10 2>&1)
  if echo "$interactive_out" | grep -q '15m' && \
     echo "$interactive_out" | grep -q "maxConcurrentSessions: 10"; then
    tap_ok "HELM-05: custom sessionTTL and maxConcurrentSessions rendered in ConfigMap"
  else
    tap_not_ok "HELM-05: custom interactive values" \
      "Custom sessionTTL or maxConcurrentSessions not rendered"
  fi

  # HELM-06: custom maxAnalyzingTimeout rendered
  interactive_out=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.interactive.enabled=true \
    --set kubernautAgent.interactive.maxAnalyzingTimeout=60m 2>&1)
  if echo "$interactive_out" | grep -q 'maxAnalyzingTimeout: "60m"'; then
    tap_ok "HELM-06: custom maxAnalyzingTimeout rendered in ConfigMap"
  else
    tap_not_ok "HELM-06: custom maxAnalyzingTimeout" \
      "Custom maxAnalyzingTimeout not rendered"
  fi

  # HELM-LINT: helm lint passes with interactive enabled
  if helm lint "$CHART_PATH" $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set kubernautAgent.interactive.enabled=true >/dev/null 2>&1; then
    tap_ok "HELM-LINT-INTERACTIVE: helm lint passes with interactive.enabled=true"
  else
    tap_not_ok "HELM-LINT-INTERACTIVE: helm lint with interactive" \
      "helm lint failed with kubernautAgent.interactive.enabled=true"
  fi

  echo "# --- Template Tests: Operator parity + OCP removal (BR-PLATFORM-003/004, Issue #1589) ---"

  # ST-CHART-MON-001: ServiceMonitor per service, gated on monitoring.serviceMonitor.enabled
  # + monitoring.coreos.com/v1 CRD presence (simulated here via --api-versions).
  local mon_sm_on
  mon_sm_on=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --api-versions monitoring.coreos.com/v1 \
    --set monitoring.serviceMonitor.enabled=true 2>&1)
  local sm_count
  sm_count=$(grep -c "^kind: ServiceMonitor$" <<< "$mon_sm_on")
  if [[ "$sm_count" -eq 10 ]]; then
    tap_ok "ST-CHART-MON-001a: 10 ServiceMonitors rendered when CRD present + enabled=true"
  else
    tap_not_ok "ST-CHART-MON-001a: ServiceMonitor count" "Expected 10 ServiceMonitors, found ${sm_count}"
  fi
  if ! grep -q "name: authwebhook-monitor" <<< "$mon_sm_on"; then
    tap_ok "ST-CHART-MON-001b: authwebhook has no ServiceMonitor (metrics intentionally disabled)"
  else
    tap_not_ok "ST-CHART-MON-001b: authwebhook ServiceMonitor" "authwebhook unexpectedly has a ServiceMonitor"
  fi

  local mon_sm_off
  mon_sm_off=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set monitoring.serviceMonitor.enabled=true 2>&1)
  if ! grep -q "^kind: ServiceMonitor$" <<< "$mon_sm_off"; then
    tap_ok "ST-CHART-MON-001c: ServiceMonitor is a no-op without the monitoring.coreos.com/v1 CRD"
  else
    tap_not_ok "ST-CHART-MON-001c: ServiceMonitor without CRD" \
      "ServiceMonitor rendered despite monitoring.coreos.com/v1 CRD not being present"
  fi

  # ST-CHART-MON-002: DataStorage/APIFrontend PrometheusRule content spot-check, same CRD gate.
  local mon_pr_on
  mon_pr_on=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --api-versions monitoring.coreos.com/v1 \
    --set monitoring.prometheusRule.enabled=true 2>&1)
  if grep -q "DataStorageDown" <<< "$mon_pr_on" && grep -q "DataStorageDLQDepthHigh" <<< "$mon_pr_on"; then
    tap_ok "ST-CHART-MON-002a: DataStorage PrometheusRule alert rules rendered"
  else
    tap_not_ok "ST-CHART-MON-002a: DataStorage PrometheusRule" "expected DataStorage alert rules not found"
  fi
  if grep -q "ApifrontendDown" <<< "$mon_pr_on" && grep -q "ApifrontendCircuitBreakerOpenKA" <<< "$mon_pr_on"; then
    tap_ok "ST-CHART-MON-002b: APIFrontend PrometheusRule alert rules rendered"
  else
    tap_not_ok "ST-CHART-MON-002b: APIFrontend PrometheusRule" "expected APIFrontend alert rules not found"
  fi

  local mon_pr_off
  mon_pr_off=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set monitoring.prometheusRule.enabled=true 2>&1)
  if ! grep -q "DataStorageDown\|ApifrontendDown" <<< "$mon_pr_off"; then
    tap_ok "ST-CHART-MON-002c: DataStorage/APIFrontend PrometheusRule is a no-op without the CRD"
  else
    tap_not_ok "ST-CHART-MON-002c: PrometheusRule without CRD" \
      "DataStorage/APIFrontend alert rules rendered despite monitoring.coreos.com/v1 CRD not being present"
  fi

  # ST-CHART-ACM-001: ACM backend tokenSecretRef wiring (BR-PLATFORM-003, #1556).
  # The Helm chart itself has no fail() guard for a missing tokenSecretRef — backend=acm
  # without a token must still render cleanly at the template layer. Enforcement is done
  # Go-side: FleetConfig.Validate() now hard-rejects backend=acm without TokenPath, so the
  # rendered Pod will fail to start (fail-closed), even though `helm template` succeeds here.
  local acm_with_token
  acm_with_token=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set gateway.fleet.enabled=true --set gateway.fleet.backend=acm \
    --set gateway.fleet.mcpGatewayEndpoint=https://mcp.example.com \
    --set gateway.fleet.tokenSecretRef=acm-token 2>&1)
  if grep -q 'tokenPath: "/etc/gateway/acm-token/token"' <<< "$acm_with_token" && \
     grep -q "fleet-acm-token" <<< "$acm_with_token"; then
    tap_ok "ST-CHART-ACM-001a: gateway.fleet.tokenSecretRef renders tokenPath + Secret volume/mount"
  else
    tap_not_ok "ST-CHART-ACM-001a: ACM tokenSecretRef wiring" \
      "tokenPath or fleet-acm-token volume/mount not found with backend=acm + tokenSecretRef set"
  fi

  local acm_without_token acm_without_token_exit
  acm_without_token=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set gateway.fleet.enabled=true --set gateway.fleet.backend=acm \
    --set gateway.fleet.mcpGatewayEndpoint=https://mcp.example.com 2>&1)
  acm_without_token_exit=$?
  if [[ "$acm_without_token_exit" -eq 0 ]] && ! grep -q "fleet-acm-token" <<< "$acm_without_token"; then
    tap_ok "ST-CHART-ACM-001b: backend=acm without tokenSecretRef renders cleanly (fails Go-side Validate() at pod startup, per #1556)"
  else
    tap_not_ok "ST-CHART-ACM-001b: backend=acm without tokenSecretRef" \
      "render failed, or fleet-acm-token volume unexpectedly present without tokenSecretRef set"
  fi

  # ST-CHART-ACM-002: same ACM backend tokenSecretRef wiring, ported to RemediationOrchestrator
  # (BR-PLATFORM-003, #1556). Mirrors ST-CHART-ACM-001 — this wiring point had zero
  # test coverage despite being identical in shape to Gateway's.
  local ro_acm_with_token
  ro_acm_with_token=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set remediationorchestrator.fleet.enabled=true --set remediationorchestrator.fleet.backend=acm \
    --set remediationorchestrator.fleet.mcpGatewayEndpoint=https://mcp.example.com \
    --set remediationorchestrator.fleet.tokenSecretRef=acm-token 2>&1)
  if grep -q 'tokenPath: "/etc/remediationorchestrator/acm-token/token"' <<< "$ro_acm_with_token" && \
     grep -q "fleet-acm-token" <<< "$ro_acm_with_token"; then
    tap_ok "ST-CHART-ACM-002a: remediationorchestrator.fleet.tokenSecretRef renders tokenPath + Secret volume/mount"
  else
    tap_not_ok "ST-CHART-ACM-002a: RemediationOrchestrator ACM tokenSecretRef wiring" \
      "tokenPath or fleet-acm-token volume/mount not found with backend=acm + tokenSecretRef set"
  fi

  local ro_acm_without_token ro_acm_without_token_exit
  ro_acm_without_token=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set remediationorchestrator.fleet.enabled=true --set remediationorchestrator.fleet.backend=acm \
    --set remediationorchestrator.fleet.mcpGatewayEndpoint=https://mcp.example.com 2>&1)
  ro_acm_without_token_exit=$?
  if [[ "$ro_acm_without_token_exit" -eq 0 ]] && ! grep -q "fleet-acm-token" <<< "$ro_acm_without_token"; then
    tap_ok "ST-CHART-ACM-002b: RemediationOrchestrator backend=acm without tokenSecretRef renders cleanly (fails Go-side Validate() at pod startup, per #1556)"
  else
    tap_not_ok "ST-CHART-ACM-002b: RemediationOrchestrator backend=acm without tokenSecretRef" \
      "render failed, or fleet-acm-token volume unexpectedly present without tokenSecretRef set"
  fi

  # ST-CHART-HPA-001: DataStorage/APIFrontend HorizontalPodAutoscaler template rendering
  # (BR-PLATFORM-003). autoscaling/v2 is a stable core API, so — unlike ServiceMonitor/
  # PrometheusRule — no CRD gate/`--api-versions` flag is needed to exercise this at the
  # template level; the live counterpart (real object + kubectl-observed spec) is
  # ST-CHART-MON-003 in flow_a_production.
  local ds_hpa_off af_hpa_off
  ds_hpa_off=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    -s templates/datastorage/hpa.yaml 2>&1)
  af_hpa_off=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    -s templates/apifrontend/hpa.yaml 2>&1)
  if ! grep -q "kind: HorizontalPodAutoscaler" <<< "$ds_hpa_off" && \
     ! grep -q "kind: HorizontalPodAutoscaler" <<< "$af_hpa_off"; then
    tap_ok "ST-CHART-HPA-001a: no HorizontalPodAutoscaler rendered by default (autoscaling.enabled=false)"
  else
    tap_not_ok "ST-CHART-HPA-001a: HPA default-disabled" \
      "HorizontalPodAutoscaler rendered despite autoscaling.enabled defaulting to false"
  fi

  local ds_hpa_on af_hpa_on
  ds_hpa_on=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set datastorage.autoscaling.enabled=true \
    --set datastorage.autoscaling.minReplicas=2 \
    --set datastorage.autoscaling.maxReplicas=6 \
    --set datastorage.autoscaling.memoryTarget=0 \
    -s templates/datastorage/hpa.yaml 2>&1)
  af_hpa_on=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set apifrontend.autoscaling.enabled=true \
    --set apifrontend.autoscaling.cpuTarget=60 \
    -s templates/apifrontend/hpa.yaml 2>&1)
  if grep -q "name: datastorage" <<< "$ds_hpa_on" && \
     grep -q "minReplicas: 2" <<< "$ds_hpa_on" && grep -q "maxReplicas: 6" <<< "$ds_hpa_on"; then
    tap_ok "ST-CHART-HPA-001b: DataStorage HPA renders with configured minReplicas/maxReplicas"
  else
    tap_not_ok "ST-CHART-HPA-001b: DataStorage HPA values wiring" \
      "expected minReplicas: 2 / maxReplicas: 6 not found for DataStorage HPA"
  fi
  if grep -q "averageUtilization: 60" <<< "$af_hpa_on"; then
    tap_ok "ST-CHART-HPA-001c: APIFrontend HPA renders with configured cpuTarget"
  else
    tap_not_ok "ST-CHART-HPA-001c: APIFrontend HPA cpuTarget wiring" \
      "expected averageUtilization: 60 not found for APIFrontend HPA"
  fi
  if ! grep -q "name: memory" <<< "$ds_hpa_on"; then
    tap_ok "ST-CHART-HPA-001d: DataStorage HPA omits the memory metric when memoryTarget=0"
  else
    tap_not_ok "ST-CHART-HPA-001d: DataStorage HPA memoryTarget=0" \
      "memory resource metric unexpectedly present despite memoryTarget=0"
  fi

  # ST-CHART-ANSIBLE-CA-001: BR-PLATFORM-005 — Ansible/AWX private-CA support
  # (Kubernaut Operator parity). caCertSecretRef combines the inter-service CA
  # with the AAP CA into one bundle via a build-ca-bundle init container.
  local ansible_ca_on
  ansible_ca_on=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set workflowexecution.config.ansible.apiURL=https://awx.example.com \
    --set workflowexecution.config.ansible.tokenSecretRef.name=awx-token \
    --set workflowexecution.config.ansible.tokenSecretRef.key=token \
    --set workflowexecution.config.ansible.caCertSecretRef.name=aap-ca-secret \
    --set workflowexecution.config.ansible.caCertSecretRef.key=custom-ca.pem \
    -s templates/workflowexecution/workflowexecution.yaml 2>&1)
  if grep -q "name: build-ca-bundle" <<< "$ansible_ca_on" && \
     grep -q 'value: "/etc/combined-ca/ca-bundle.crt"' <<< "$ansible_ca_on" && \
     grep -q "secretName: aap-ca-secret" <<< "$ansible_ca_on"; then
    tap_ok "ST-CHART-ANSIBLE-CA-001a: ansible.caCertSecretRef wires build-ca-bundle init container + combined TLS_CA_FILE"
  else
    tap_not_ok "ST-CHART-ANSIBLE-CA-001a: ansible.caCertSecretRef wiring" \
      "build-ca-bundle init container or combined-ca TLS_CA_FILE override not found"
  fi

  local ansible_ca_off
  ansible_ca_off=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set workflowexecution.config.ansible.apiURL=https://awx.example.com \
    --set workflowexecution.config.ansible.tokenSecretRef.name=awx-token \
    --set workflowexecution.config.ansible.tokenSecretRef.key=token \
    -s templates/workflowexecution/workflowexecution.yaml 2>&1)
  if ! grep -q "build-ca-bundle" <<< "$ansible_ca_off"; then
    tap_ok "ST-CHART-ANSIBLE-CA-001b: no build-ca-bundle init container when caCertSecretRef unset"
  else
    tap_not_ok "ST-CHART-ANSIBLE-CA-001b: ansible without caCertSecretRef" \
      "build-ca-bundle init container unexpectedly present"
  fi

  # ST-CHART-ANSIBLE-NP-001: BR-PLATFORM-005 — WorkflowExecution NetworkPolicy
  # allows HTTPS egress to the (possibly external) AWX/AAP endpoint.
  local ansible_np
  ansible_np=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set workflowexecution.config.ansible.apiURL=https://awx.example.com \
    --set workflowexecution.config.ansible.tokenSecretRef.name=awx-token \
    --set workflowexecution.config.ansible.tokenSecretRef.key=token \
    -s templates/workflowexecution/networkpolicy.yaml 2>&1)
  if grep -A2 "port: 443" <<< "$ansible_np" | grep -q "protocol: TCP"; then
    tap_ok "ST-CHART-ANSIBLE-NP-001: WorkflowExecution NetworkPolicy allows HTTPS egress when ansible configured"
  else
    tap_not_ok "ST-CHART-ANSIBLE-NP-001: WorkflowExecution NetworkPolicy" \
      "expected port 443 egress rule not found when ansible configured"
  fi

  # ST-CHART-AF-NP-001: BR-PLATFORM-005 — APIFrontend NetworkPolicy (previously
  # the only component in the mesh without one; Kubernaut Operator parity).
  local af_np
  af_np=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set "networkPolicies.apifrontend.ingressNamespaces[0]=ingress-nginx" \
    --set apifrontend.config.auth.replayCache.enabled=true \
    --set apifrontend.config.auth.issuerURL=https://issuer.example.com \
    -s templates/apifrontend/networkpolicy.yaml 2>&1)
  if grep -q "kind: NetworkPolicy" <<< "$af_np" && \
     grep -q "kubernetes.io/metadata.name: ingress-nginx" <<< "$af_np" && \
     grep -A5 "port: 6379" <<< "$af_np" | grep -q "app: valkey"; then
    tap_ok "ST-CHART-AF-NP-001a: APIFrontend NetworkPolicy renders ingressNamespaces + Valkey egress"
  else
    tap_not_ok "ST-CHART-AF-NP-001a: APIFrontend NetworkPolicy" \
      "expected NetworkPolicy content not found"
  fi

  local af_np_disabled
  af_np_disabled=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set networkPolicies.apifrontend.enabled=false 2>&1)
  if ! grep -q "test-kubernaut-apifrontend" <<< "$af_np_disabled"; then
    tap_ok "ST-CHART-AF-NP-001b: networkPolicies.apifrontend.enabled=false omits the NetworkPolicy"
  else
    tap_not_ok "ST-CHART-AF-NP-001b: APIFrontend NetworkPolicy disable toggle" \
      "NetworkPolicy rendered despite networkPolicies.apifrontend.enabled=false"
  fi

  local valkey_np
  valkey_np=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set apifrontend.config.auth.replayCache.enabled=true \
    -s templates/infrastructure/networkpolicy-valkey.yaml 2>&1)
  if grep -q "app: apifrontend" <<< "$valkey_np"; then
    tap_ok "ST-CHART-AF-NP-001c: Valkey NetworkPolicy allows APIFrontend ingress when replayCache enabled"
  else
    tap_not_ok "ST-CHART-AF-NP-001c: Valkey NetworkPolicy" \
      "apifrontend podSelector not found in Valkey ingress despite replayCache.enabled=true"
  fi

  # ST-CHART-KA-SATOKEN-001: BR-PLATFORM-005 — kubernaut-agent (highest-risk,
  # LLM-driven component) uses a short-TTL projected SA token instead of the
  # default long-lived automount (Kubernaut Operator parity).
  local ka_tpl
  ka_tpl=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    -s templates/kubernaut-agent/kubernaut-agent.yaml 2>&1)
  if grep -q "automountServiceAccountToken: false" <<< "$ka_tpl" && \
     grep -q "expirationSeconds: 3600" <<< "$ka_tpl" && \
     grep -q "mountPath: /var/run/secrets/kubernetes.io/serviceaccount" <<< "$ka_tpl"; then
    tap_ok "ST-CHART-KA-SATOKEN-001: kubernaut-agent-sa disables automount + mounts short-TTL projected token"
  else
    tap_not_ok "ST-CHART-KA-SATOKEN-001: kubernaut-agent SA token hardening" \
      "expected automount disable + projected sa-token volume not found"
  fi

  # ST-CHART-KA-RBAC-001: BR-PLATFORM-005 — generic (non-OCP) investigative RBAC
  # parity: KubeVirt VM investigation + PriorityClass visibility.
  if grep -q '"kubevirt.io"' <<< "$ka_tpl" && grep -q '"scheduling.k8s.io"' <<< "$ka_tpl" && \
     grep -q '"priorityclasses"' <<< "$ka_tpl"; then
    tap_ok "ST-CHART-KA-RBAC-001: kubernaut-agent ClusterRole includes kubevirt.io + priorityclasses read rules"
  else
    tap_not_ok "ST-CHART-KA-RBAC-001: kubernaut-agent RBAC parity" \
      "kubevirt.io or scheduling.k8s.io/priorityclasses rules not found"
  fi

  # ST-CHART-CONSOLE-001a: BR-PLATFORM-006 — console disabled by default (opt-in),
  # matching the Kubernaut Operator's ConsoleSpec.Enabled default of false.
  local console_default
  console_default=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) 2>&1)
  if ! grep -q "app: console" <<< "$console_default"; then
    tap_ok "ST-CHART-CONSOLE-001a: console is disabled (not rendered) by default"
  else
    tap_not_ok "ST-CHART-CONSOLE-001a: console default-disabled" \
      "console resources rendered despite console.enabled defaulting to false"
  fi

  # ST-CHART-CONSOLE-001b: fail-fast validation — console.enabled=true without
  # console.auth.secretName must fail the render, not silently misconfigure OIDC.
  local console_no_secret
  console_no_secret=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set console.enabled=true 2>&1)
  if grep -q "console.auth.secretName is required" <<< "$console_no_secret"; then
    tap_ok "ST-CHART-CONSOLE-001b: console.enabled=true without auth.secretName fails fast"
  else
    tap_not_ok "ST-CHART-CONSOLE-001b: console auth.secretName validation" \
      "expected fail-fast error not found"
  fi

  # ST-CHART-CONSOLE-001c: fail-fast validation — console.enabled=true without an
  # OIDC issuer resolvable from APIFrontend's auth config must fail the render.
  local console_no_issuer
  console_no_issuer=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set console.enabled=true \
    --set console.auth.secretName=console-oauth-creds \
    --set console.ingress.host=console.apps.example.com 2>&1)
  if grep -q "requires an OIDC issuer" <<< "$console_no_issuer"; then
    tap_ok "ST-CHART-CONSOLE-001c: console.enabled=true without a resolvable OIDC issuer fails fast"
  else
    tap_not_ok "ST-CHART-CONSOLE-001c: console OIDC issuer validation" \
      "expected fail-fast error not found"
  fi

  # ST-CHART-CONSOLE-001d: fail-fast validation — console.enabled=true without
  # console.ingress.host must fail (oauth2-proxy redirect URL requires a hostname
  # even when console.ingress.enabled=false, since it may be fronted by a
  # user-managed Ingress/Route instead).
  local console_no_host
  console_no_host=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set console.enabled=true \
    --set console.auth.secretName=console-oauth-creds \
    --set apifrontend.config.auth.issuerURL=https://issuer.example.com 2>&1)
  if grep -q "console.ingress.host is required" <<< "$console_no_host"; then
    tap_ok "ST-CHART-CONSOLE-001d: console.enabled=true without ingress.host fails fast"
  else
    tap_not_ok "ST-CHART-CONSOLE-001d: console ingress.host validation" \
      "expected fail-fast error not found"
  fi

  # ST-CHART-CONSOLE-002: full happy-path render — Deployment (oauth2-proxy +
  # console containers), Service, nginx ConfigMap, Ingress, and NetworkPolicy all
  # render with the required fields set (Kubernaut Operator parity, Issue #1589
  # follow-up).
  local console_full
  console_full=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set console.enabled=true \
    --set console.auth.secretName=console-oauth-creds \
    --set console.ingress.host=console.apps.example.com \
    --set apifrontend.config.auth.issuerURL=https://issuer.example.com 2>&1)
  if grep -q "name: oauth2-proxy" <<< "$console_full" && \
     grep -q "name: console" <<< "$console_full" && \
     grep -q "kind: Ingress" <<< "$console_full" && \
     grep -q "host: console.apps.example.com" <<< "$console_full" && \
     grep -q "oidc-issuer-url=https://issuer.example.com" <<< "$console_full" && \
     grep -q "redirect-url=https://console.apps.example.com/oauth2/callback" <<< "$console_full" && \
     grep -q "name: test-kubernaut-console" <<< "$console_full"; then
    tap_ok "ST-CHART-CONSOLE-002: console renders Deployment+Service+Ingress+NetworkPolicy end-to-end"
  else
    tap_not_ok "ST-CHART-CONSOLE-002: console full render" \
      "expected console resources/content not found"
  fi

  # ST-CHART-CONSOLE-003: console.ingress.enabled=false omits the Ingress while
  # the Deployment/Service still render (external access delegated to a
  # user-managed Ingress/Route).
  local console_no_ingress
  console_no_ingress=$(helm template test "$CHART_PATH" \
    $(template_common_args) $(template_llm_args) $(policy_flags) \
    --set console.enabled=true \
    --set console.auth.secretName=console-oauth-creds \
    --set console.ingress.host=console.apps.example.com \
    --set console.ingress.enabled=false \
    --set apifrontend.config.auth.issuerURL=https://issuer.example.com 2>&1)
  if ! grep -q "kind: Ingress" <<< "$console_no_ingress" && grep -q "name: oauth2-proxy" <<< "$console_no_ingress"; then
    tap_ok "ST-CHART-CONSOLE-003: console.ingress.enabled=false omits Ingress, keeps Deployment/Service"
  else
    tap_not_ok "ST-CHART-CONSOLE-003: console ingress disable toggle" \
      "Ingress still rendered, or Deployment unexpectedly missing"
  fi

  # ST-CHART-OCP-REMOVED-001: Part B regression guard — no residual OCP-specific code paths.
  # Excludes generic schema example strings and comments pointing OCP users to the Operator.
  local ocp_hits
  ocp_hits=$(grep -rlI "postgresql\.variant\|kubernaut\.monitoring\.isOCP\|kubernaut\.monitoring\.ocpRbac\|service\.beta\.openshift\.io\|cluster-monitoring-view\|values-ocp\.yaml" \
    "${CHART_PATH}/templates" "${CHART_PATH}/values.yaml" "${CHART_PATH}/values.schema.json" 2>/dev/null || true)
  if [[ -z "$ocp_hits" ]] && [[ ! -f "${CHART_PATH}/values-ocp.yaml" ]]; then
    tap_ok "ST-CHART-OCP-REMOVED-001: no residual OCP-specific helpers/annotations/RBAC/values-ocp.yaml"
  else
    tap_not_ok "ST-CHART-OCP-REMOVED-001: OCP removal regression" \
      "residual OCP-specific code found: ${ocp_hits}"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  echo "# Helm Chart Smoke Tests"
  echo "# Platform: ${PLATFORM}"
  echo "# Image tag: ${IMAGE_TAG}"
  echo "# Registry: ${IMAGE_REGISTRY:-default}"
  echo "# Pull secret: ${PULL_SECRET:-none}"
  echo "# Chart: ${CHART_PATH}"
  echo "# Namespace: ${NAMESPACE}"
  echo "# TLS mode: ${TLS_MODE}"
  echo "#"

  setup_policy_files

  tap_header

  # Template tests run first (no cluster required)
  run_template_tests

  # Always start clean
  full_cleanup

  # Pre-load the Helm hook image (bitnami/kubectl) into Kind so that
  # pre-delete hooks don't hang on Docker Hub rate limits during uninstall.
  preload_hook_image

  if [[ "$TLS_MODE" == "cert-manager" ]]; then
    flow_c_cert_manager
  else
    flow_a_production

    if [[ "$PLATFORM" == "kind" ]]; then
      full_cleanup
      flow_b_quickstart
    fi
  fi

  tap_footer

  cleanup_port_forward

  if [[ "$TAP_FAIL" -gt 0 ]]; then
    echo "# RESULT: FAIL (${TAP_FAIL} failures)"
    if [[ -n "$MUST_GATHER_DIR" && -d "$MUST_GATHER_DIR" ]]; then
      echo "# Must-gather archive: ${MUST_GATHER_DIR}.tar.gz"
      echo "# Upload this artifact for offline triage."
    fi
    exit 1
  else
    echo "# RESULT: PASS (${TAP_PASS} passed)"
    exit 0
  fi
}

cleanup_all() {
  cleanup_port_forward
  cleanup_policy_files
}

trap cleanup_all EXIT
main
