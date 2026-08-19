#!/usr/bin/env bash
# Kubernaut Must-Gather - Bats Test Helpers
# BR-PLATFORM-001.3.4: Testing framework utilities

# Test directories
# Anchored to this file's own location (not BATS_TEST_DIRNAME, which is the
# CALLING test file's directory and varies by nesting depth -- e.g.
# test/integration/test_e2e.bats sits one level deeper than test/*.bats,
# which silently resolved MUST_GATHER_ROOT to test/ instead of the actual
# cmd/must-gather/ root there, breaking every ${MUST_GATHER_ROOT}/gather.sh
# call in that file with "No such file or directory").
export MUST_GATHER_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export TEST_TEMP_DIR="${BATS_TEST_TMPDIR}/must-gather-test"
export MOCK_COLLECTION_DIR="${TEST_TEMP_DIR}/collection"

# Source paths - check if running in container (installed paths) or locally (source paths)
if [ -d "/usr/share/must-gather/collectors" ]; then
    # Running in container - use installed paths
    export COLLECTORS_DIR="/usr/share/must-gather/collectors"
    export SANITIZERS_DIR="/usr/share/must-gather/sanitizers"
    export UTILS_DIR="/usr/share/must-gather/utils"
    export GATHER_SCRIPT="/usr/bin/gather"
else
    # Running locally - use source paths
    export COLLECTORS_DIR="${MUST_GATHER_ROOT}/collectors"
    export SANITIZERS_DIR="${MUST_GATHER_ROOT}/sanitizers"
    export UTILS_DIR="${MUST_GATHER_ROOT}/utils"
    export GATHER_SCRIPT="${MUST_GATHER_ROOT}/gather.sh"
fi

# Mock kubectl responses
export MOCK_KUBECTL_BIN="${TEST_TEMP_DIR}/bin/kubectl"

# Setup function - called before each test
setup_test_environment() {
    # Create test directories
    mkdir -p "${TEST_TEMP_DIR}"
    mkdir -p "${MOCK_COLLECTION_DIR}"
    mkdir -p "${TEST_TEMP_DIR}/bin"

    # Set test namespaces (Issue #2037: single-release-namespace model, no more
    # obsolete kubernaut-notifications entry -- see gather.sh RELEASE_NAMESPACE/
    # WORKFLOW_NAMESPACE)
    export RELEASE_NAMESPACE="kubernaut-system"
    export WORKFLOW_NAMESPACE="kubernaut-workflows"

    # Set test configuration
    export SINCE_DURATION="24h"
    export DEST_DIR="${TEST_TEMP_DIR}"
    export SANITIZE_ENABLED="true"
    export MAX_SIZE_MB="500"
}

# Teardown function - called after each test
teardown_test_environment() {
    # Clean up test directories
    if [ -d "${TEST_TEMP_DIR}" ]; then
        rm -rf "${TEST_TEMP_DIR}"
    fi
}

# Mock kubectl command
mock_kubectl() {
    local response_file="$1"

    cat > "${MOCK_KUBECTL_BIN}" <<EOF
#!/bin/bash
# Mock kubectl for testing

# Debug: Write all calls to a log file
echo "\$(date +%H:%M:%S) kubectl \$@" >> ${TEST_TEMP_DIR}/kubectl-calls.log

# Check for dynamic CRD discovery query (Issue #2037: "kubectl get crd -o name",
# no specific CRD name in the args -- must be checked before the single-CRD
# definition branch below, which always includes a specific ".kubernaut.ai" name)
if [[ "\$*" == "get crd -o name"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/crd-list.txt" ]; then
        cat "${TEST_TEMP_DIR}/crd-list.txt"
        exit 0
    else
        exit 0
    fi
fi

# Check for CRD definition query
if [[ "\$*" == *"get crd"* ]] && [[ "\$*" == *".kubernaut.ai"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/crd-def.yaml" ]; then
        cat "${TEST_TEMP_DIR}/crd-def.yaml"
        exit 0
    else
        echo "---"
        exit 1
    fi
fi

# Check for CRD instances query (must check before general "get" to avoid conflicts)
if [[ "\$*" == "get "* ]] && [[ "\$*" == *".kubernaut.ai --all-namespaces -o yaml"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/crd-instances.yaml" ]; then
        cat "${TEST_TEMP_DIR}/crd-instances.yaml"
        exit 0
    else
        echo "---"
        exit 1
    fi
fi

# Check for instance count query
if [[ "\$*" == "get "* ]] && [[ "\$*" == *".kubernaut.ai --all-namespaces --no-headers"* ]]; then
    echo "test-rr-001  kubernaut-system"
    exit 0
fi

# Check for the optional kubernaut-operator-system namespace (Issue #2037:
# the separate kubernaut-operator component's namespace -- absent unless a
# test explicitly marks it present via create_mock_operator_pod_names_list).
# Must be checked before the generic "get namespace "* branch below.
if [[ "\$*" == "get namespace kubernaut-operator-system"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/operator-ns-present" ]; then
        echo "Active"
        exit 0
    else
        exit 1
    fi
fi

# Check for namespace existence (used by logs.sh before listing pods).
# Defaults to "exists" for any namespace name -- tests that need a "missing
# namespace" scenario override this via a dedicated non-matching kubectl mock.
if [[ "\$*" == "get namespace "* ]]; then
    echo "Active"
    exit 0
fi

# Check for operator-namespace pod-name listing (Issue #2037: kubectl get
# pods -n kubernaut-operator-system --no-headers -- must be checked before
# the generic pod-name branch below, which serves RELEASE_NAMESPACE's fixture).
if [[ "\$*" == *"get pods -n kubernaut-operator-system --no-headers"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/operator-pod-names.txt" ]; then
        cat "${TEST_TEMP_DIR}/operator-pod-names.txt"
        exit 0
    else
        exit 0
    fi
fi

# Check for pod-name listing (Issue #2037: "kubectl get pods -n NS --no-headers",
# used by logs.sh's all-pod discovery -- must be checked before the general
# "get pods" YAML branch below, which is a different output shape)
if [[ "\$*" == *"get pods"* ]] && [[ "\$*" == *"--no-headers"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/pod-names.txt" ]; then
        cat "${TEST_TEMP_DIR}/pod-names.txt"
        exit 0
    else
        exit 0
    fi
fi

# Check for pods (full YAML PodList)
if [[ "\$*" == *"get pods"* ]]; then
    cat "${TEST_TEMP_DIR}/pod-list.yaml"
    exit 0
fi

# Check for events
if [[ "\$*" == *"get events"* ]]; then
    cat "${TEST_TEMP_DIR}/events.yaml"
    exit 0
fi

# Check for Job name listing (must-gather jobs.sh collector, Issue #2036):
# "kubectl get jobs -n WORKFLOW_NAMESPACE --no-headers" -- must be checked
# before the bulk "get jobs ... -o yaml" branch below, which is a different
# output shape (plural "jobs", same as the no-headers query).
if [[ "\$*" == *"get jobs"* ]] && [[ "\$*" == *"--no-headers"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/job-names.txt" ]; then
        cat "${TEST_TEMP_DIR}/job-names.txt"
        exit 0
    else
        exit 0
    fi
fi

# Check for bulk Jobs listing (must-gather jobs.sh collector, Issue #2036):
# "kubectl get jobs -n WORKFLOW_NAMESPACE -o yaml" (plural -- all Jobs).
if [[ "\$*" == "get jobs "* ]] && [[ "\$*" == *"-o yaml"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/jobs-list.yaml" ]; then
        cat "${TEST_TEMP_DIR}/jobs-list.yaml"
        exit 0
    else
        echo "---"
        exit 1
    fi
fi

# Check for single Job spec (must-gather jobs.sh collector, Issue #2036):
# "kubectl get job NAME -n WORKFLOW_NAMESPACE -o yaml" (singular -- one Job).
if [[ "\$*" == "get job "* ]] && [[ "\$*" == *"-o yaml"* ]]; then
    if [ -f "${TEST_TEMP_DIR}/job-spec.yaml" ]; then
        cat "${TEST_TEMP_DIR}/job-spec.yaml"
        exit 0
    else
        echo "---"
        exit 1
    fi
fi

# Check for logs
if [[ "\$*" == *"logs"* ]]; then
    echo "Mock log output for testing"
    exit 0
fi

# Check for version
if [[ "\$*" == *"version"* ]]; then
    echo "Client Version: v1.31.0"
    exit 0
fi

# Check for current context (used by gather.sh's collection-metadata.json
# cluster_name field). Must return a clean single-line value on success --
# falling through to the generic "---"+exit1 default below would embed a
# literal newline in the JSON string ("---\nunknown"), corrupting it.
if [[ "\$*" == *"current-context"* ]]; then
    echo "test-cluster"
    exit 0
fi

# Default: return empty for other commands
echo "---"
exit 1
EOF

    chmod +x "${MOCK_KUBECTL_BIN}"
    export PATH="${TEST_TEMP_DIR}/bin:${PATH}"
}

# Create mock CRD response
create_mock_crd_response() {
    # Issue #2037: crds.sh now discovers CRD types dynamically via
    # "kubectl get crd -o name" before collecting each one's definition/
    # instances -- register this fixture's own CRD in that discovery list by
    # default so existing single-CRD-focused tests keep working unless they
    # explicitly override the list via create_mock_crd_list.
    if [ ! -f "${TEST_TEMP_DIR}/crd-list.txt" ]; then
        echo "customresourcedefinition.apiextensions.k8s.io/remediationrequests.kubernaut.ai" \
            > "${TEST_TEMP_DIR}/crd-list.txt"
    fi

    # Create CRD definition response (for kubectl get crd)
    cat > "${TEST_TEMP_DIR}/crd-def.yaml" <<'EOF'
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: remediationrequests.kubernaut.ai
spec:
  group: kubernaut.ai
  names:
    kind: RemediationRequest
    plural: remediationrequests
EOF

    # Create CRD instances response (for kubectl get remediationrequests.kubernaut.ai)
    cat > "${TEST_TEMP_DIR}/crd-instances.yaml" <<'EOF'
apiVersion: v1
kind: List
items:
  - apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    metadata:
      name: test-rr-001
      namespace: kubernaut-system
    spec:
      signal_id: "test-signal"
    status:
      phase: "Completed"
EOF

    # Backward compatibility: keep crd-response.yaml for other uses
    cat > "${TEST_TEMP_DIR}/crd-response.yaml" <<'EOF'
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: remediationrequests.kubernaut.ai
spec:
  group: kubernaut.ai
  names:
    kind: RemediationRequest
    plural: remediationrequests
EOF
}

# Create mock "kubectl get crd -o name" response (Issue #2037: dynamic CRD
# discovery). Includes types absent from the old static 6-entry allowlist
# (actiontypes, effectivenessassessments) plus one unrelated non-kubernaut CRD,
# to prove both completeness (UT-MG-2037-001) and regex precision (R3).
create_mock_crd_list() {
    cat > "${TEST_TEMP_DIR}/crd-list.txt" <<'EOF'
customresourcedefinition.apiextensions.k8s.io/remediationrequests.kubernaut.ai
customresourcedefinition.apiextensions.k8s.io/aianalyses.kubernaut.ai
customresourcedefinition.apiextensions.k8s.io/actiontypes.kubernaut.ai
customresourcedefinition.apiextensions.k8s.io/effectivenessassessments.kubernaut.ai
customresourcedefinition.apiextensions.k8s.io/widgets.example.com
EOF
}

# Create mock pod list response
create_mock_pod_list() {
    cat > "${TEST_TEMP_DIR}/pod-list.yaml" <<'EOF'
apiVersion: v1
kind: PodList
items:
- metadata:
    name: gateway-abc123
    namespace: kubernaut-system
  status:
    phase: Running
- metadata:
    name: datastorage-xyz789
    namespace: kubernaut-system
  status:
    phase: Running
EOF
}

# Create mock "kubectl get pods -n NS --no-headers" plain-text response
# (Issue #2037: logs.sh all-pod discovery). Includes pods for services absent
# from the old SERVICE_PATTERNS allowlist (authwebhook, apifrontend) to prove
# the allowlist removal actually took effect (UT-MG-2037-003).
create_mock_pod_names_list() {
    cat > "${TEST_TEMP_DIR}/pod-names.txt" <<'EOF'
gateway-abc123   1/1   Running   0   5m
datastorage-xyz789   1/1   Running   0   5m
authwebhook-abc123   1/1   Running   0   5m
apifrontend-xyz789   1/1   Running   0   5m
EOF
}

# Mark the optional kubernaut-operator-system namespace as present on the
# (mocked) cluster and seed its pod list (Issue #2037: the separate
# kubernaut-operator component's controller-manager pod, a THIRD namespace
# outside RELEASE_NAMESPACE/WORKFLOW_NAMESPACE -- see UT-MG-2037-005).
create_mock_operator_pod_names_list() {
    touch "${TEST_TEMP_DIR}/operator-ns-present"
    cat > "${TEST_TEMP_DIR}/operator-pod-names.txt" <<'EOF'
kubernaut-operator-controller-manager-abc123   1/1   Running   0   5m
EOF
}

# Create mock "kubectl get jobs -n WORKFLOW_NAMESPACE --no-headers" plain-text
# response (Issue #2036: jobs.sh Job-name discovery, mirrors
# create_mock_pod_names_list's column shape).
create_mock_job_names_list() {
    cat > "${TEST_TEMP_DIR}/job-names.txt" <<'EOF'
wfe-deployment-frontend-abc123   1/1   3m   5m
wfe-statefulset-cache-xyz789   0/1   -    2m
EOF
}

# Create mock "kubectl get jobs -n WORKFLOW_NAMESPACE -o yaml" bulk response
# (Issue #2036: jobs.sh all-jobs.yaml content).
create_mock_jobs_list_yaml() {
    cat > "${TEST_TEMP_DIR}/jobs-list.yaml" <<'EOF'
apiVersion: v1
kind: List
items:
- apiVersion: batch/v1
  kind: Job
  metadata:
    name: wfe-deployment-frontend-abc123
    namespace: kubernaut-workflows
  status:
    failed: 1
    conditions:
    - type: Failed
      status: "True"
      reason: BackoffLimitExceeded
- apiVersion: batch/v1
  kind: Job
  metadata:
    name: wfe-statefulset-cache-xyz789
    namespace: kubernaut-workflows
  status:
    active: 1
EOF
}

# Create mock "kubectl get job NAME -n WORKFLOW_NAMESPACE -o yaml" single-Job
# response (Issue #2036: jobs.sh per-Job spec.yaml content).
create_mock_job_spec_yaml() {
    cat > "${TEST_TEMP_DIR}/job-spec.yaml" <<'EOF'
apiVersion: batch/v1
kind: Job
metadata:
  name: wfe-deployment-frontend-abc123
  namespace: kubernaut-workflows
spec:
  backoffLimit: 0
status:
  failed: 1
  conditions:
  - type: Failed
    status: "True"
    reason: BackoffLimitExceeded
    message: "Job has reached the specified backoff limit"
EOF
}

# Create mock events response
create_mock_events() {
    cat > "${TEST_TEMP_DIR}/events.yaml" <<'EOF'
apiVersion: v1
kind: EventList
items:
- metadata:
    name: test-event-1
    namespace: kubernaut-system
  type: Normal
  reason: Started
  message: "Container started"
  lastTimestamp: "2025-01-04T12:00:00Z"
EOF
}

# Assert file exists
assert_file_exists() {
    local file="$1"
    if [ ! -f "${file}" ]; then
        echo "Expected file does not exist: ${file}"
        return 1
    fi
}

# Assert directory exists
assert_directory_exists() {
    local dir="$1"
    if [ ! -d "${dir}" ]; then
        echo "Expected directory does not exist: ${dir}"
        return 1
    fi
}

# Assert file contains pattern
assert_file_contains() {
    local file="$1"
    local pattern="$2"

    if [ ! -f "${file}" ]; then
        echo "File does not exist: ${file}"
        return 1
    fi

    # Use grep -F for fixed string matching (no regex interpretation of special chars like [])
    if ! grep -qF "${pattern}" "${file}"; then
        echo "File ${file} does not contain pattern: ${pattern}"
        return 1
    fi
}

# Assert file does NOT contain pattern (for sanitization tests)
assert_file_not_contains() {
    local file="$1"
    local pattern="$2"

    if [ ! -f "${file}" ]; then
        echo "File does not exist: ${file}"
        return 1
    fi

    # Use grep -F for fixed string matching (no regex interpretation of special chars like [])
    if grep -qF "${pattern}" "${file}"; then
        echo "File ${file} should NOT contain pattern: ${pattern}"
        return 1
    fi
}

# Count files matching pattern
count_files() {
    local directory="$1"
    local pattern="$2"

    find "${directory}" -name "${pattern}" 2>/dev/null | wc -l
}

# Mock curl for DataStorage API tests
mock_curl() {
    local response_file="$1"

    cat > "${TEST_TEMP_DIR}/bin/curl" <<EOF
#!/bin/bash
# Mock curl for testing
# Issue #2037: record the invoked URL so tests can assert DATASTORAGE_URL
# was built from the configured RELEASE_NAMESPACE (last arg is the URL).
echo "\${@: -1}" >> "${TEST_TEMP_DIR}/curl-calls.log"
cat "${response_file}"
exit 0
EOF

    chmod +x "${TEST_TEMP_DIR}/bin/curl"
    export PATH="${TEST_TEMP_DIR}/bin:${PATH}"
}

# Create mock DataStorage API response
create_mock_datastorage_workflows() {
    cat > "${TEST_TEMP_DIR}/workflows.json" <<'EOF'
{
  "workflows": [
    {"name": "workflow-1", "status": "active"},
    {"name": "workflow-2", "status": "inactive"}
  ],
  "total": 2
}
EOF
}

create_mock_datastorage_audit() {
    cat > "${TEST_TEMP_DIR}/audit-events.json" <<'EOF'
{
  "data": [
    {"event_type": "remediation.created", "timestamp": "2026-01-04T12:00:00Z"},
    {"event_type": "workflow.executed", "timestamp": "2026-01-04T12:05:00Z"}
  ],
  "pagination": {"total": 2, "limit": 1000, "offset": 0}
}
EOF
}

# Verify script exit code and output
assert_success() {
    if [ "$status" -ne 0 ]; then
        echo "Expected success (exit 0), got: $status"
        echo "Output: $output"
        return 1
    fi
}

assert_failure() {
    if [ "$status" -eq 0 ]; then
        echo "Expected failure (exit non-zero), got success"
        echo "Output: $output"
        return 1
    fi
}

