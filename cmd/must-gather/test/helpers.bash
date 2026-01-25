#!/usr/bin/env bash
# Kubernaut Must-Gather - Bats Test Helpers
# BR-PLATFORM-001.3.4: Testing framework utilities

# Test directories
export MUST_GATHER_ROOT="${BATS_TEST_DIRNAME}/.."
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

    # Set test namespace list
    export KUBERNAUT_NAMESPACES=("kubernaut-system" "kubernaut-notifications" "kubernaut-workflows")

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

# Check for pods
if [[ "\$*" == *"get pods"* ]]; then
    cat "${TEST_TEMP_DIR}/pod-list.yaml"
    exit 0
fi

# Check for events
if [[ "\$*" == *"get events"* ]]; then
    cat "${TEST_TEMP_DIR}/events.yaml"
    exit 0
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

# Default: return empty for other commands
echo "---"
exit 1
EOF

    chmod +x "${MOCK_KUBECTL_BIN}"
    export PATH="${TEST_TEMP_DIR}/bin:${PATH}"
}

# Create mock CRD response
create_mock_crd_response() {
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

