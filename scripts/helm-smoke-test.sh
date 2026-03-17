#!/usr/bin/env bash
# Helm Chart Smoke Test Runner
# Authority: docs/testing/342/TEST_PLAN.md
# Output: TAP v13 (Test Anything Protocol)
#
# Usage:
#   ./scripts/helm-smoke-test.sh --platform kind --image-tag 1.0.1-rc1-arm64 --chart-path charts/kubernaut/
#   ./scripts/helm-smoke-test.sh --platform ocp  --image-tag 1.0.1-rc1-amd64 --chart-path charts/kubernaut/
#
# Flows executed per platform:
#   Kind: Flow A (production lifecycle with hook TLS) + Flow B (dev quick start)
#   OCP:  Flow A (production lifecycle with cert-manager TLS)

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
CERT_MANAGER_ISSUER="selfsigned-issuer"

# TAP state
TAP_COUNT=0
TAP_PASS=0
TAP_FAIL=0

# Port-forward PID tracking
PF_PID=""

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
    -h|--help)
      echo "Usage: $0 --platform kind|ocp --image-tag TAG --chart-path PATH [--registry REGISTRY] [--pull-secret NAME]"
      echo ""
      echo "Options:"
      echo "  --platform    Target platform: kind or ocp (default: kind)"
      echo "  --image-tag   Container image tag (required)"
      echo "  --registry    Container image registry (overrides global.image.registry)"
      echo "  --pull-secret Kubernetes docker-registry secret name for private registries"
      echo "  --chart-path  Path to chart directory (default: charts/kubernaut/)"
      echo "  --namespace   Kubernetes namespace (default: kubernaut-system)"
      echo "  --timeout     Pod readiness timeout (default: 300s)"
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [[ -z "$IMAGE_TAG" ]]; then
  echo "Error: --image-tag is required"
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
  flags+=" --set effectivenessmonitor.external.prometheusEnabled=false"
  flags+=" --set effectivenessmonitor.external.alertManagerEnabled=false"
  if [[ "$PLATFORM" == "kind" ]]; then
    flags+=" --set global.image.pullPolicy=IfNotPresent"
  fi
  echo "$flags"
}

tls_flags() {
  if [[ "$PLATFORM" == "ocp" ]]; then
    echo "--set tls.mode=cert-manager --set tls.certManager.issuerRef.name=${CERT_MANAGER_ISSUER}"
  else
    echo "--set tls.mode=manual"
  fi
}

production_secret_flags() {
  echo "--set postgresql.auth.existingSecret=kubernaut-pg-credentials"
  echo "--set datastorage.dbExistingSecret=kubernaut-ds-credentials"
  echo "--set valkey.existingSecret=kubernaut-valkey-credentials"
  echo "--set holmesgptApi.llm.provider=openai"
  echo "--set holmesgptApi.llm.model=gpt-4o"
  echo "--set holmesgptApi.llm.credentialsSecretName=kubernaut-llm-credentials"
  echo "--set gateway.auth.signalSources[0].name=alertmanager"
  echo "--set gateway.auth.signalSources[0].serviceAccount=alertmanager-kube-prometheus-stack-alertmanager"
  echo "--set gateway.auth.signalSources[0].namespace=monitoring"
}

# ---------------------------------------------------------------------------
# Scenario implementations
# ---------------------------------------------------------------------------

run_pre_001() {
  local desc="ST-CHART-PRE-001: Install CRDs"
  if kubectl apply --server-side --force-conflicts -f "${CHART_PATH}/crds/" >/dev/null 2>&1; then
    local count
    count=$(kubectl get crds 2>/dev/null | grep -c "kubernaut.ai" || true)
    if [[ "$count" -eq 9 ]]; then
      tap_ok "$desc (9 CRDs created)"
    else
      tap_not_ok "$desc" "Expected 9 CRDs, found ${count}"
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

  kubectl create secret generic kubernaut-pg-credentials \
    --from-literal=POSTGRES_USER=slm_user \
    --from-literal=POSTGRES_PASSWORD="$test_password" \
    --from-literal=POSTGRES_DB=action_history \
    -n "$NAMESPACE" >/dev/null 2>&1 || pass=false

  kubectl create secret generic kubernaut-ds-credentials \
    --from-literal="db-secrets.yaml=username: slm_user
password: ${test_password}" \
    -n "$NAMESPACE" >/dev/null 2>&1 || pass=false

  kubectl create secret generic kubernaut-valkey-credentials \
    --from-literal="valkey-secrets.yaml=password: \"${test_password}\"" \
    -n "$NAMESPACE" >/dev/null 2>&1 || pass=false

  kubectl create secret generic kubernaut-llm-credentials --from-literal=OPENAI_API_KEY=sk-smoke-test-placeholder -n "$NAMESPACE" >/dev/null 2>&1 || pass=false # pre-commit:allow-sensitive

  local secret_count=4
  if [[ -n "$PULL_SECRET" && -n "${PULL_SECRET_SERVER:-}" ]]; then
    kubectl create secret docker-registry "$PULL_SECRET" \
      --docker-server="$PULL_SECRET_SERVER" \
      --docker-username="${PULL_SECRET_USER:-}" \
      --docker-password="${PULL_SECRET_TOKEN:-}" \
      -n "$NAMESPACE" >/dev/null 2>&1 || pass=false
    secret_count=5
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
  assert_pods_ready 12
}

run_verify_002() {
  assert_port_forward_responds \
    "holmesgpt-api" 8080 "/health" \
    "ST-CHART-VERIFY-002: HolmesGPT API health endpoint"
}

run_verify_003() {
  assert_port_forward_responds \
    "data-storage-service" 8081 "/health" \
    "ST-CHART-VERIFY-003: DataStorage health endpoint"
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

run_tls_002() {
  local cert_ready
  cert_ready=$(kubectl get certificate authwebhook-cert -n "$NAMESPACE" \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
  if [[ "$cert_ready" == "True" ]]; then
    tap_ok "ST-CHART-TLS-002a: Certificate authwebhook-cert Ready=True"
  else
    tap_not_ok "ST-CHART-TLS-002a: Certificate authwebhook-cert Ready=True" "status: ${cert_ready}"
  fi

  assert_resource_exists secret authwebhook-tls "$NAMESPACE" \
    "ST-CHART-TLS-002b: authwebhook-tls Secret provisioned by cert-manager"
}

run_tls_003() {
  local desc_delete="ST-CHART-TLS-003a: Delete authwebhook-tls Secret"
  assert_exit_code "$desc_delete" kubectl delete secret authwebhook-tls -n "$NAMESPACE"

  local flags
  flags="$(common_install_flags) $(tls_flags)"

  # shellcheck disable=SC2046
  if helm upgrade kubernaut "$CHART_PATH" \
    --namespace "$NAMESPACE" \
    $(production_secret_flags) \
    $flags \
    --timeout 5m >/dev/null 2>&1; then
    tap_ok "ST-CHART-TLS-003b: helm upgrade regenerates certificate"
  else
    tap_not_ok "ST-CHART-TLS-003b: helm upgrade regenerates certificate" "helm upgrade failed"
  fi

  assert_resource_exists secret authwebhook-tls "$NAMESPACE" \
    "ST-CHART-TLS-003c: authwebhook-tls Secret exists after recovery"
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

  assert_pods_ready 12 "ST-CHART-UPG-001d: 12 pods healthy after upgrade"
}

run_uninst_001() {
  # The chart's pre-delete hook (webhook-cleanup Job) removes admission webhooks
  # before Helm deletes the release resources, preventing failurePolicy=Fail
  # rejections when the authwebhook pod terminates before demo-content CRs.
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
  if [[ "$crd_count" -eq 9 ]]; then
    tap_ok "ST-CHART-UNINST-001d: 9 CRDs retained"
  else
    tap_not_ok "ST-CHART-UNINST-001d: 9 CRDs retained" "Found ${crd_count} CRDs"
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
  hook_image=$(grep -A1 'tlsCerts:' "$CHART_PATH/values.yaml" | grep 'image:' | awk '{print $2}' | head -1)

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

  if [[ "$PLATFORM" == "kind" ]]; then
    run_tls_001
  else
    run_tls_002
  fi

  run_upg_001 || flow_failed=true

  if [[ "$PLATFORM" == "ocp" ]]; then
    run_tls_003
  fi

  run_edge_001

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
  run_edge_001

  if $flow_failed; then
    must_gather "$NAMESPACE" "flow-b-verification-failure"
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
  echo "--set" "datastorage.dbExistingSecret=dummy"
  echo "--set" "valkey.existingSecret=dummy"
}

template_llm_args() {
  echo "--set" "holmesgptApi.llm.provider=openai"
  echo "--set" "holmesgptApi.llm.model=gpt-4"
}

run_template_tests() {
  echo "# --- Template Tests: Issue #390 ConfigMap Split ---"
  local tpl_flag="-s"
  local tpl_path="templates/holmesgpt-api/holmesgpt-api.yaml"
  local output
  local tier2_file
  tier2_file=$(mktemp)
  trap "rm -f '$tier2_file'" RETURN

  cat > "$tier2_file" <<'SDKEOF'
llm:
  provider: anthropic
  model: claude-4
  endpoint: http://custom-llm:8080
toolsets:
  prometheus/metrics:
    enabled: true
    config:
      prometheus_url: http://prom:9090
SDKEOF

  # IT-HAPI-390-001: Two ConfigMaps rendered
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) 2>&1)
  if echo "$output" | grep -q "name: holmesgpt-api-config" && \
     echo "$output" | grep -q "name: holmesgpt-sdk-config"; then
    tap_ok "IT-HAPI-390-001: helm template renders both holmesgpt-api-config and holmesgpt-sdk-config"
  else
    tap_not_ok "IT-HAPI-390-001: helm template renders both ConfigMaps" "Missing one or both ConfigMaps in output"
  fi

  # IT-HAPI-390-002: existingSdkConfigMap skips SDK template
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) \
    --set holmesgptApi.existingSdkConfigMap=my-custom 2>&1)
  if ! echo "$output" | grep -q "name: holmesgpt-sdk-config" && \
     echo "$output" | grep -q 'name: my-custom'; then
    tap_ok "IT-HAPI-390-002: existingSdkConfigMap skips SDK ConfigMap, references user ConfigMap"
  else
    tap_not_ok "IT-HAPI-390-002: existingSdkConfigMap skips SDK template" "holmesgpt-sdk-config still rendered or user ConfigMap not referenced"
  fi

  # IT-HAPI-390-003: Deployment has sdk-config volume mount
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) 2>&1)
  if echo "$output" | grep -q "mountPath: /etc/holmesgpt/sdk" && \
     echo "$output" | grep -q "name: sdk-config"; then
    tap_ok "IT-HAPI-390-003: Deployment has sdk-config volume and /etc/holmesgpt/sdk mount"
  else
    tap_not_ok "IT-HAPI-390-003: Deployment sdk-config volume mount" "Missing sdk-config volume or mount"
  fi

  # IT-HAPI-390-004: helm lint passes
  if helm lint "$CHART_PATH" $(template_common_args) $(template_llm_args) >/dev/null 2>&1 && \
     helm lint "$CHART_PATH" $(template_common_args) $(template_llm_args) --set holmesgptApi.existingSdkConfigMap=my-custom >/dev/null 2>&1; then
    tap_ok "IT-HAPI-390-004: helm lint passes for default and existingSdkConfigMap modes"
  else
    tap_not_ok "IT-HAPI-390-004: helm lint" "One or more lint modes failed"
  fi

  echo "# --- Template Tests: SDK Auto-Generated Defaults ---"

  # Auto-generated config: toolsets empty by default
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) 2>&1)
  if echo "$output" | grep -A1 "toolsets:" | grep -q "{}"; then
    tap_ok "ST-SDK-DEFAULTS-001: auto-generated config renders toolsets: {}"
  else
    tap_not_ok "ST-SDK-DEFAULTS-001: auto-generated defaults" "Expected empty toolsets"
  fi

  echo "# --- Template Tests: SDK Config Tiers ---"

  # Tier 2: sdkConfigContent renders verbatim via --set-file
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) \
    --set-file "holmesgptApi.sdkConfigContent=$tier2_file" 2>&1)
  if echo "$output" | grep -q "provider: anthropic" && \
     echo "$output" | grep -q "model: claude-4"; then
    tap_ok "ST-SDK-TIER2-001: sdkConfigContent renders user content verbatim via --set-file"
  else
    tap_not_ok "ST-SDK-TIER2-001: sdkConfigContent verbatim" "User content not found in output"
  fi

  # Tier 2: sdkConfigContent suppresses auto-generated values
  if ! echo "$output" | grep -q "max_retries:"; then
    tap_ok "ST-SDK-TIER2-002: sdkConfigContent suppresses auto-generated structured values"
  else
    tap_not_ok "ST-SDK-TIER2-002: sdkConfigContent suppresses auto-gen" "Auto-generated max_retries still present"
  fi

  # Tier 3 wins over Tier 2: existingSdkConfigMap takes priority
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) \
    --set-file "holmesgptApi.sdkConfigContent=$tier2_file" \
    --set holmesgptApi.existingSdkConfigMap=external-cm 2>&1)
  if ! echo "$output" | grep -q "name: holmesgpt-sdk-config" && \
     echo "$output" | grep -q "name: external-cm"; then
    tap_ok "ST-SDK-TIER3-001: existingSdkConfigMap takes priority over sdkConfigContent"
  else
    tap_not_ok "ST-SDK-TIER3-001: existingSdkConfigMap priority" "ConfigMap still rendered or external-cm not referenced"
  fi

  echo "# --- Template Tests: GCP Conditional Fields ---"

  # GCP fields conditional: not rendered for non-vertex providers
  output=$(helm template test "$CHART_PATH" "$tpl_flag" "$tpl_path" \
    $(template_common_args) $(template_llm_args) 2>&1)
  if ! echo "$output" | grep -q "gcp_project_id"; then
    tap_ok "ST-SDK-GCP-001: gcp_project_id not rendered for non-vertex provider"
  else
    tap_not_ok "ST-SDK-GCP-001: gcp_project_id conditional" "gcp_project_id rendered for openai provider"
  fi

  # Note: Vertex AI / GCP-specific fields (gcpProjectId, gcpRegion) are configured
  # via sdkConfigContent or existingSdkConfigMap, not auto-generated quickstart config.
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
  echo "#"

  tap_header

  # Template tests run first (no cluster required)
  run_template_tests

  # Always start clean
  full_cleanup

  # Pre-load the Helm hook image (bitnami/kubectl) into Kind so that
  # pre-delete hooks don't hang on Docker Hub rate limits during uninstall.
  preload_hook_image

  flow_a_production

  if [[ "$PLATFORM" == "kind" ]]; then
    full_cleanup
    flow_b_quickstart
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

trap cleanup_port_forward EXIT
main
