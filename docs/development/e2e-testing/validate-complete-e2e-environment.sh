#!/bin/bash
# Complete E2E Testing Environment Validation Script
# Validates all components of the Kubernaut E2E testing environment

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-kubernaut-e2e}"
AI_MODEL_ENDPOINT="${AI_MODEL_ENDPOINT:-http://localhost:8080}"
AI_MODEL_NAME="${AI_MODEL_NAME:-gpt-oss:20b}"
DETAILED_VALIDATION="${DETAILED_VALIDATION:-false}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-300}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Status counters
CHECKS_PASSED=0
CHECKS_FAILED=0
CHECKS_WARNING=0
CHECKS_TOTAL=0

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $1"; ((CHECKS_PASSED++)); ((CHECKS_TOTAL++)); }
log_warning() { echo -e "${YELLOW}[WARN]${NC} $1"; ((CHECKS_WARNING++)); ((CHECKS_TOTAL++)); }
log_error() { echo -e "${RED}[FAIL]${NC} $1"; ((CHECKS_FAILED++)); ((CHECKS_TOTAL++)); }
log_header() { echo -e "\n${PURPLE}=== $1 ===${NC}"; }

# Banner
echo -e "${PURPLE}"
cat << "EOF"
 _  __     _                                 _     _____ ____  _____   _   _       _ _     _       _   _
| |/ /    | |                               | |   |  ___/ ___||  ___| | | | |     | (_)   | |     | | (_)
| ' /_   _| |__   ___ _ __ _ __   __ _ _   _  | |_  | |__ \___ \| |__   | | | | __ _| |_  __| | __ _| |_ _  ___  _ __
|  <| | | | '_ \ / _ \ '__| '_ \ / _` | | | | | __| |  __| ___) |  __|  | | | |/ _` | | |/ _` |/ _` | __| |/ _ \| '_ \
| . \ |_| | |_) |  __/ |  | | | | (_| | |_| | | |_  | |___\____/| |___  \ \_/ / (_| | | | (_| | (_| | |_| | (_) | | | |
|_|\_\__,_|_.__/ \___|_|  |_| |_|\__,_|\__,_|  \__| \____/     \____/   \___/ \__,_|_|_|\__,_|\__,_|\__|_|\___/|_| |_|
EOF
echo -e "${NC}"

log_info "Kubernaut Complete E2E Testing Environment Validation"
log_info "Cluster: ${CLUSTER_NAME}"
log_info "AI Model: ${AI_MODEL_NAME} @ ${AI_MODEL_ENDPOINT}"
log_info "Detailed Validation: ${DETAILED_VALIDATION}"
echo ""

# Utility functions
check_command() {
    local cmd="$1"
    local description="$2"

    if command -v "$cmd" &>/dev/null; then
        log_success "$description is available"
        return 0
    else
        log_error "$description is not available"
        return 1
    fi
}

check_url_connectivity() {
    local url="$1"
    local description="$2"
    local timeout="${3:-10}"

    if curl -s --connect-timeout "$timeout" "$url" >/dev/null 2>&1; then
        log_success "$description is accessible"
        return 0
    else
        log_error "$description is not accessible"
        return 1
    fi
}

wait_for_condition() {
    local condition_cmd="$1"
    local description="$2"
    local timeout="${3:-120}"
    local interval="${4:-5}"

    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        if eval "$condition_cmd" >/dev/null 2>&1; then
            log_success "$description"
            return 0
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_error "$description (timeout after ${timeout}s)"
    return 1
}

# Validation functions

# 1. Prerequisites Validation
validate_prerequisites() {
    log_header "Prerequisites Validation"

    # Check required commands
    check_command "kcli" "KCLI"
    check_command "oc" "OpenShift CLI"
    check_command "kubectl" "Kubernetes CLI" || log_warning "kubectl not found (oc can be used instead)"
    check_command "curl" "curl"
    check_command "jq" "jq JSON processor" || log_warning "jq not found (some validations may be limited)"

    # Check authentication files
    local pull_secret_path ssh_key_path
    pull_secret_path=$(grep -oP "pull_secret: '\K[^']*" "${SCRIPT_DIR}/kcli-baremetal-params.yml" 2>/dev/null | sed "s|~|$HOME|" || echo "")
    ssh_key_path=$(grep -oP "ssh_key: '\K[^']*" "${SCRIPT_DIR}/kcli-baremetal-params.yml" 2>/dev/null | sed "s|~|$HOME|" || echo "")

    if [[ -f "$pull_secret_path" ]]; then
        log_success "Pull secret found: $pull_secret_path"
    else
        log_warning "Pull secret not found or not configured"
    fi

    if [[ -f "$ssh_key_path" ]]; then
        log_success "SSH key found: $ssh_key_path"
    else
        log_warning "SSH key not found or not configured"
    fi
}

# 2. OpenShift Cluster Validation
validate_cluster() {
    log_header "OpenShift Cluster Validation"

    # Check if cluster exists in KCLI
    if kcli list cluster | grep -q "^${CLUSTER_NAME}"; then
        log_success "Cluster exists in KCLI: ${CLUSTER_NAME}"

        # Get cluster status
        local cluster_status
        cluster_status=$(kcli info cluster "${CLUSTER_NAME}" 2>/dev/null | grep -i "status" || echo "unknown")
        log_info "Cluster status: $cluster_status"
    else
        log_error "Cluster not found in KCLI: ${CLUSTER_NAME}"
        return 1
    fi

    # Check kubeconfig
    local kubeconfig_path="$HOME/.kcli/clusters/${CLUSTER_NAME}/auth/kubeconfig"
    if [[ -f "$kubeconfig_path" ]]; then
        log_success "Kubeconfig found: $kubeconfig_path"
        export KUBECONFIG="$kubeconfig_path"
    else
        log_error "Kubeconfig not found: $kubeconfig_path"
        return 1
    fi

    # Test cluster connectivity
    if oc whoami &>/dev/null; then
        log_success "Cluster authentication successful"
        local username
        username=$(oc whoami 2>/dev/null || echo "unknown")
        log_info "Authenticated as: $username"
    else
        log_error "Cannot authenticate with cluster"
        return 1
    fi

    # Check cluster nodes
    local total_nodes ready_nodes
    total_nodes=$(oc get nodes --no-headers 2>/dev/null | wc -l || echo "0")
    ready_nodes=$(oc get nodes --no-headers 2>/dev/null | grep -c "Ready" || echo "0")

    if [[ $total_nodes -gt 0 ]]; then
        log_success "Cluster nodes: $ready_nodes/$total_nodes Ready"
        if [[ $ready_nodes -eq $total_nodes ]]; then
            log_success "All cluster nodes are Ready"
        else
            log_warning "Some cluster nodes are not Ready"
        fi
    else
        log_error "No cluster nodes found"
        return 1
    fi

    # Check cluster operators
    if oc get co &>/dev/null; then
        local total_operators available_operators
        total_operators=$(oc get co --no-headers 2>/dev/null | wc -l || echo "0")
        available_operators=$(oc get co --no-headers 2>/dev/null | grep -c "True.*False.*False" || echo "0")

        log_info "Cluster operators: $available_operators/$total_operators Available"
        if [[ $available_operators -eq $total_operators ]]; then
            log_success "All cluster operators are Available"
        else
            log_warning "Some cluster operators are not Available"
        fi
    else
        log_warning "Cannot check cluster operators"
    fi
}

# 3. Storage Infrastructure Validation
validate_storage() {
    log_header "Storage Infrastructure Validation"

    # Check storage classes
    if oc get storageclass &>/dev/null; then
        local storage_classes default_sc
        storage_classes=$(oc get storageclass --no-headers 2>/dev/null | wc -l || echo "0")
        default_sc=$(oc get storageclass 2>/dev/null | grep "(default)" | awk '{print $1}' || echo "none")

        if [[ $storage_classes -gt 0 ]]; then
            log_success "Storage classes found: $storage_classes"
            log_info "Default storage class: $default_sc"
        else
            log_error "No storage classes found"
        fi
    else
        log_error "Cannot access storage classes"
    fi

    # Check Local Storage Operator
    if oc get namespace openshift-local-storage &>/dev/null; then
        if oc get csv -n openshift-local-storage | grep -q "local-storage-operator.*Succeeded"; then
            log_success "Local Storage Operator is running"
        else
            log_warning "Local Storage Operator may not be ready"
        fi
    else
        log_warning "Local Storage Operator namespace not found"
    fi

    # Check OpenShift Data Foundation
    if oc get namespace openshift-storage &>/dev/null; then
        if oc get csv -n openshift-storage | grep -q "odf-operator.*Succeeded"; then
            log_success "OpenShift Data Foundation operator is running"

            # Check storage cluster
            if oc get storagecluster ocs-storagecluster -n openshift-storage &>/dev/null; then
                local cluster_phase
                cluster_phase=$(oc get storagecluster ocs-storagecluster -n openshift-storage -o jsonpath='{.status.phase}' 2>/dev/null || echo "unknown")
                if [[ "$cluster_phase" == "Ready" ]]; then
                    log_success "ODF Storage Cluster is Ready"
                else
                    log_warning "ODF Storage Cluster phase: $cluster_phase"
                fi
            else
                log_warning "ODF Storage Cluster not found"
            fi
        else
            log_warning "OpenShift Data Foundation operator may not be ready"
        fi
    else
        log_warning "OpenShift Data Foundation namespace not found"
    fi

    # Test storage functionality
    if [[ "$DETAILED_VALIDATION" == "true" ]]; then
        log_info "Testing storage functionality..."
        cat > /tmp/test-pvc.yaml << EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: validation-test-pvc
  namespace: default
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF

        if oc apply -f /tmp/test-pvc.yaml &>/dev/null; then
            if wait_for_condition "oc get pvc validation-test-pvc -n default -o jsonpath='{.status.phase}' | grep -q Bound" "Storage test PVC bound" 60; then
                log_success "Storage functionality test passed"
            else
                log_warning "Storage functionality test failed"
            fi
            oc delete pvc validation-test-pvc -n default &>/dev/null || true
        else
            log_warning "Could not create test PVC"
        fi
        rm -f /tmp/test-pvc.yaml
    fi
}

# 4. AI Model Integration Validation
validate_ai_model() {
    log_header "AI Model Integration Validation"

    # Test AI model connectivity
    if check_url_connectivity "$AI_MODEL_ENDPOINT/v1/models" "AI model endpoint"; then
        # Check if specific model is available
        if curl -s --connect-timeout 10 "$AI_MODEL_ENDPOINT/v1/models" > /tmp/models.json 2>/dev/null; then
            if command -v jq >/dev/null 2>&1; then
                if jq -e ".data[] | select(.id == \"$AI_MODEL_NAME\")" /tmp/models.json &>/dev/null; then
                    log_success "AI model '$AI_MODEL_NAME' is available"
                else
                    log_warning "AI model '$AI_MODEL_NAME' not found"
                    log_info "Available models:"
                    jq -r '.data[].id' /tmp/models.json 2>/dev/null | head -5 | sed 's/^/  - /'
                fi
            elif grep -q "$AI_MODEL_NAME" /tmp/models.json; then
                log_success "AI model '$AI_MODEL_NAME' is available"
            else
                log_warning "AI model '$AI_MODEL_NAME' not found in response"
            fi
            rm -f /tmp/models.json
        else
            log_error "Could not retrieve model list from AI endpoint"
        fi
    fi

    # Test AI model functionality
    if [[ "$DETAILED_VALIDATION" == "true" ]]; then
        log_info "Testing AI model functionality..."
        local test_response
        test_response=$(curl -s --connect-timeout 10 -X POST "$AI_MODEL_ENDPOINT/v1/chat/completions" \
            -H "Content-Type: application/json" \
            -d "{
                \"model\": \"$AI_MODEL_NAME\",
                \"messages\": [{\"role\": \"user\", \"content\": \"Test connection\"}],
                \"temperature\": 0.3,
                \"max_tokens\": 50
            }" 2>/dev/null || echo "")

        if [[ -n "$test_response" ]] && echo "$test_response" | grep -q "choices"; then
            log_success "AI model functionality test passed"
        else
            log_warning "AI model functionality test failed"
        fi
    fi

    # Check AI model configuration in cluster
    if oc get configmap kubernaut-ai-config -n kubernaut-system &>/dev/null; then
        log_success "AI model configuration found in cluster"
    else
        log_warning "AI model configuration not found in cluster"
    fi
}

# 5. Vector Database Validation
validate_vector_database() {
    log_header "Vector Database Validation"

    # Check kubernaut-system namespace
    if oc get namespace kubernaut-system &>/dev/null; then
        log_success "Kubernaut system namespace exists"
    else
        log_error "Kubernaut system namespace not found"
        return 1
    fi

    # Check PostgreSQL deployment
    if oc get deployment postgresql-vector -n kubernaut-system &>/dev/null; then
        if wait_for_condition "oc get deployment postgresql-vector -n kubernaut-system -o jsonpath='{.status.readyReplicas}' | grep -q 1" "PostgreSQL vector database ready" 30; then
            log_success "PostgreSQL vector database is running"
        else
            log_warning "PostgreSQL vector database may not be ready"
        fi
    else
        log_error "PostgreSQL vector database deployment not found"
        return 1
    fi

    # Check PostgreSQL service
    if oc get service postgresql-vector -n kubernaut-system &>/dev/null; then
        log_success "PostgreSQL vector database service exists"
    else
        log_warning "PostgreSQL vector database service not found"
    fi

    # Test database connectivity and pgvector extension
    if [[ "$DETAILED_VALIDATION" == "true" ]]; then
        log_info "Testing vector database functionality..."
        local db_test_result
        db_test_result=$(oc exec -n kubernaut-system deployment/postgresql-vector -- psql -U kubernaut -d kubernaut -c "SELECT 1;" 2>/dev/null || echo "failed")

        if [[ "$db_test_result" != "failed" ]]; then
            log_success "Database connectivity test passed"

            # Test pgvector extension
            local vector_test_result
            vector_test_result=$(oc exec -n kubernaut-system deployment/postgresql-vector -- psql -U kubernaut -d kubernaut -c "SELECT * FROM pg_extension WHERE extname = 'vector';" 2>/dev/null || echo "failed")

            if [[ "$vector_test_result" != "failed" ]] && echo "$vector_test_result" | grep -q "vector"; then
                log_success "pgvector extension is installed"
            else
                log_warning "pgvector extension test failed"
            fi
        else
            log_warning "Database connectivity test failed"
        fi
    fi
}

# 6. Kubernaut Application Validation
validate_kubernaut_application() {
    log_header "Kubernaut Application Validation"

    # Check Kubernaut deployment
    if oc get deployment kubernaut -n kubernaut-system &>/dev/null; then
        if wait_for_condition "oc get deployment kubernaut -n kubernaut-system -o jsonpath='{.status.readyReplicas}' | grep -q 1" "Kubernaut application ready" 60; then
            log_success "Kubernaut application is running"
        else
            log_warning "Kubernaut application may not be ready"
        fi
    else
        log_warning "Kubernaut application deployment not found"
    fi

    # Check Kubernaut service
    if oc get service kubernaut-service -n kubernaut-system &>/dev/null; then
        log_success "Kubernaut service exists"
    else
        log_warning "Kubernaut service not found"
    fi

    # Check Kubernaut RBAC
    if oc get serviceaccount kubernaut -n kubernaut-system &>/dev/null; then
        log_success "Kubernaut service account exists"
    else
        log_warning "Kubernaut service account not found"
    fi

    if oc get clusterrole kubernaut &>/dev/null; then
        log_success "Kubernaut cluster role exists"
    else
        log_warning "Kubernaut cluster role not found"
    fi

    if oc get clusterrolebinding kubernaut &>/dev/null; then
        log_success "Kubernaut cluster role binding exists"
    else
        log_warning "Kubernaut cluster role binding not found"
    fi

    # Test Kubernaut health endpoint
    if [[ "$DETAILED_VALIDATION" == "true" ]]; then
        log_info "Testing Kubernaut health endpoint..."
        if oc get pods -n kubernaut-system -l app=kubernaut --no-headers | grep -q "Running"; then
            local health_test
            health_test=$(oc exec -n kubernaut-system deployment/kubernaut -- curl -s http://localhost:8081/health 2>/dev/null || echo "failed")

            if [[ "$health_test" != "failed" ]] && echo "$health_test" | grep -q "healthy\|ok"; then
                log_success "Kubernaut health endpoint test passed"
            else
                log_warning "Kubernaut health endpoint test failed"
            fi
        else
            log_warning "No running Kubernaut pods found for health test"
        fi
    fi
}

# 7. LitmusChaos Framework Validation
validate_chaos_testing() {
    log_header "LitmusChaos Framework Validation"

    # Check Litmus namespace
    if oc get namespace litmus &>/dev/null; then
        log_success "Litmus namespace exists"

        # Check Litmus operator
        if oc get pods -n litmus -l app.kubernetes.io/name=litmus --no-headers | grep -q "Running"; then
            log_success "LitmusChaos operator is running"
        else
            log_warning "LitmusChaos operator may not be running"
        fi
    else
        log_warning "Litmus namespace not found"
    fi

    # Check chaos testing namespace
    if oc get namespace chaos-testing &>/dev/null; then
        log_success "Chaos testing namespace exists"
    else
        log_warning "Chaos testing namespace not found"
    fi

    # Check chaos experiments
    if oc get chaosexperiments -n chaos-testing &>/dev/null; then
        local experiment_count
        experiment_count=$(oc get chaosexperiments -n chaos-testing --no-headers 2>/dev/null | wc -l || echo "0")
        if [[ $experiment_count -gt 0 ]]; then
            log_success "Chaos experiments available: $experiment_count"
        else
            log_warning "No chaos experiments found"
        fi
    else
        log_warning "Cannot access chaos experiments"
    fi

    # Check chaos testing RBAC
    if oc get serviceaccount chaos-e2e-runner -n chaos-testing &>/dev/null; then
        log_success "Chaos testing service account exists"
    else
        log_warning "Chaos testing service account not found"
    fi

    if oc get clusterrole chaos-e2e-runner &>/dev/null; then
        log_success "Chaos testing cluster role exists"
    else
        log_warning "Chaos testing cluster role not found"
    fi
}

# 8. Monitoring Stack Validation
validate_monitoring() {
    log_header "Monitoring Stack Validation"

    # Check monitoring namespace
    if oc get namespace kubernaut-monitoring &>/dev/null; then
        log_success "Monitoring namespace exists"

        # Check Prometheus deployment
        if oc get deployment prometheus -n kubernaut-monitoring &>/dev/null; then
            if wait_for_condition "oc get deployment prometheus -n kubernaut-monitoring -o jsonpath='{.status.readyReplicas}' | grep -q 1" "Prometheus ready" 30; then
                log_success "Prometheus is running"
            else
                log_warning "Prometheus may not be ready"
            fi
        else
            log_info "Custom Prometheus deployment not found"
        fi
    else
        log_info "Custom monitoring namespace not found"
    fi

    # Check OpenShift cluster monitoring
    if oc get namespace openshift-monitoring &>/dev/null; then
        log_success "OpenShift cluster monitoring is available"

        # Check if ServiceMonitor exists for Kubernaut
        if oc get servicemonitor kubernaut-metrics -n kubernaut-system &>/dev/null; then
            log_success "Kubernaut ServiceMonitor exists"
        else
            log_info "Kubernaut ServiceMonitor not found"
        fi
    else
        log_warning "OpenShift cluster monitoring not available"
    fi

    # Test monitoring endpoint
    if [[ "$DETAILED_VALIDATION" == "true" ]]; then
        if oc get service prometheus -n kubernaut-monitoring &>/dev/null; then
            log_info "Testing Prometheus endpoint..."
            local prometheus_test
            prometheus_test=$(oc exec -n kubernaut-monitoring deployment/prometheus -- curl -s http://localhost:9090/-/healthy 2>/dev/null || echo "failed")

            if [[ "$prometheus_test" != "failed" ]] && echo "$prometheus_test" | grep -q "Prometheus is Healthy"; then
                log_success "Prometheus health test passed"
            else
                log_warning "Prometheus health test failed"
            fi
        fi
    fi
}

# 9. Test Applications Validation
validate_test_applications() {
    log_header "Test Applications Validation"

    # Check test applications namespace
    if oc get namespace kubernaut-test-apps &>/dev/null; then
        log_success "Test applications namespace exists"

        # Check test deployments
        local test_apps=("memory-intensive-app" "cpu-intensive-app" "database-app")
        local ready_apps=0
        local total_apps=${#test_apps[@]}

        for app in "${test_apps[@]}"; do
            if oc get deployment "$app" -n kubernaut-test-apps &>/dev/null; then
                if oc get deployment "$app" -n kubernaut-test-apps -o jsonpath='{.status.readyReplicas}' | grep -q "^[1-9]"; then
                    log_success "Test application '$app' is ready"
                    ((ready_apps++))
                else
                    log_warning "Test application '$app' is not ready"
                fi
            else
                log_warning "Test application '$app' not found"
            fi
        done

        log_info "Test applications ready: $ready_apps/$total_apps"
    else
        log_info "Test applications namespace not found"
    fi

    # Check test patterns in database
    if [[ "$DETAILED_VALIDATION" == "true" ]]; then
        log_info "Checking test patterns in vector database..."
        local pattern_count
        pattern_count=$(oc exec -n kubernaut-system deployment/postgresql-vector -- psql -U kubernaut -d kubernaut -t -c "SELECT COUNT(*) FROM patterns;" 2>/dev/null | tr -d ' ' || echo "0")

        if [[ "$pattern_count" -gt 0 ]]; then
            log_success "Test patterns found in database: $pattern_count"
        else
            log_warning "No test patterns found in database"
        fi
    fi
}

# 10. End-to-End Integration Test
validate_integration() {
    log_header "End-to-End Integration Validation"

    if [[ "$DETAILED_VALIDATION" != "true" ]]; then
        log_info "Detailed validation disabled - skipping integration tests"
        return 0
    fi

    log_info "Running basic integration test..."

    # Test full pipeline: AI model -> Kubernaut -> Vector DB
    local integration_test_passed=true

    # 1. Test AI model response
    if ! check_url_connectivity "$AI_MODEL_ENDPOINT/v1/models" "AI model endpoint" 5; then
        integration_test_passed=false
    fi

    # 2. Test vector database connectivity
    if ! oc exec -n kubernaut-system deployment/postgresql-vector -- psql -U kubernaut -d kubernaut -c "SELECT 1;" &>/dev/null; then
        log_warning "Vector database connectivity test failed"
        integration_test_passed=false
    fi

    # 3. Test Kubernaut health
    if ! oc get pods -n kubernaut-system -l app=kubernaut --no-headers | grep -q "Running"; then
        log_warning "Kubernaut application not running"
        integration_test_passed=false
    fi

    if [[ "$integration_test_passed" == "true" ]]; then
        log_success "Basic integration test passed"
    else
        log_warning "Basic integration test failed"
    fi
}

# Print validation summary
print_validation_summary() {
    log_header "Validation Summary"

    local success_rate=0
    if [[ $CHECKS_TOTAL -gt 0 ]]; then
        success_rate=$(( (CHECKS_PASSED * 100) / CHECKS_TOTAL ))
    fi

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  VALIDATION SUMMARY${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo -e "Passed:   ${GREEN}${CHECKS_PASSED}${NC}"
    echo -e "Warnings: ${YELLOW}${CHECKS_WARNING}${NC}"
    echo -e "Failed:   ${RED}${CHECKS_FAILED}${NC}"
    echo -e "Total:    ${CHECKS_TOTAL}"
    echo -e "Success Rate: ${success_rate}%"

    echo -e "\n${BLUE}Environment Status:${NC}"
    if [[ $CHECKS_FAILED -eq 0 ]]; then
        if [[ $CHECKS_WARNING -eq 0 ]]; then
            echo -e "${GREEN}✓ Environment is fully ready for E2E testing!${NC}"
        else
            echo -e "${YELLOW}⚠ Environment is ready with warnings${NC}"
            echo -e "  Review warnings above for optimal performance"
        fi

        echo -e "\n${BLUE}Ready for E2E Testing:${NC}"
        echo -e "  • Run tests: ./run-e2e-tests.sh all"
        echo -e "  • Specific use case: ./run-use-case-1.sh"
        echo -e "  • Chaos testing: ./run-e2e-tests.sh chaos"
        echo -e "  • Stress testing: ./run-e2e-tests.sh stress"

    else
        echo -e "${RED}✗ Environment has critical issues${NC}"
        echo -e "  Please fix failed checks before running E2E tests"

        echo -e "\n${BLUE}Common Solutions:${NC}"
        echo -e "  • Check cluster status: oc get nodes"
        echo -e "  • Verify AI model: curl $AI_MODEL_ENDPOINT/v1/models"
        echo -e "  • Redeploy components: ./setup-complete-e2e-environment.sh"
    fi

    echo -e "${GREEN}========================================${NC}\n"

    # Exit with appropriate code
    if [[ $CHECKS_FAILED -eq 0 ]]; then
        exit 0
    else
        exit 1
    fi
}

# Main execution function
main() {
    log_info "Starting comprehensive validation of Kubernaut E2E testing environment"
    echo ""

    # Execute all validation steps
    validate_prerequisites
    validate_cluster
    validate_storage
    validate_ai_model
    validate_vector_database
    validate_kubernaut_application
    validate_chaos_testing
    validate_monitoring
    validate_test_applications
    validate_integration

    # Print summary
    print_validation_summary
}

# Handle script arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --detailed)
            DETAILED_VALIDATION="true"
            shift
            ;;
        --cluster-name)
            CLUSTER_NAME="$2"
            shift 2
            ;;
        --ai-endpoint)
            AI_MODEL_ENDPOINT="$2"
            shift 2
            ;;
        --ai-model)
            AI_MODEL_NAME="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT_SECONDS="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --detailed           Run detailed validation tests"
            echo "  --cluster-name NAME  Specify cluster name"
            echo "  --ai-endpoint URL    Specify AI model endpoint"
            echo "  --ai-model NAME      Specify AI model name"
            echo "  --timeout SECONDS    Timeout for waiting operations"
            echo "  --help               Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  CLUSTER_NAME         Cluster name to validate"
            echo "  AI_MODEL_ENDPOINT    AI model endpoint URL"
            echo "  AI_MODEL_NAME        AI model name"
            echo "  DETAILED_VALIDATION  Run detailed tests (true/false)"
            echo "  TIMEOUT_SECONDS      Timeout for operations"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            log_info "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Check if script is being sourced or executed
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
