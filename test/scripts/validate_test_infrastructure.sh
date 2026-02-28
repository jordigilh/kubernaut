#!/bin/bash
# Infrastructure Validation Script for Kubernaut Test Environment
# Validates all required infrastructure components before running tests

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Kubernaut Test Infrastructure Validation${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

ERROR_COUNT=0
WARNING_COUNT=0

# Function to check command availability
check_command() {
    local cmd=$1
    local required=$2

    if command -v "$cmd" &> /dev/null; then
        echo -e "${GREEN}✅${NC} $cmd: $(command -v $cmd)"
        return 0
    else
        if [ "$required" = "true" ]; then
            echo -e "${RED}❌${NC} $cmd: NOT FOUND (required)"
            ERROR_COUNT=$((ERROR_COUNT + 1))
        else
            echo -e "${YELLOW}⚠️${NC} $cmd: NOT FOUND (optional)"
            WARNING_COUNT=$((WARNING_COUNT + 1))
        fi
        return 1
    fi
}

# Function to check service accessibility
check_service() {
    local service_name=$1
    local host=$2
    local port=$3

    if timeout 2 bash -c "echo > /dev/tcp/$host/$port" 2>/dev/null; then
        echo -e "${GREEN}✅${NC} $service_name: accessible at $host:$port"
        return 0
    else
        echo -e "${RED}❌${NC} $service_name: NOT accessible at $host:$port"
        ERROR_COUNT=$((ERROR_COUNT + 1))
        return 1
    fi
}

# ==========================================
# Check Required Commands
# ==========================================
echo -e "${BLUE}Checking required commands...${NC}"

check_command "go" "true"
check_command "kubectl" "true"
check_command "podman" "true"
check_command "kind" "true"

echo ""

# ==========================================
# Check Optional Commands
# ==========================================
echo -e "${BLUE}Checking optional commands...${NC}"

check_command "docker" "false"
check_command "helm" "false"

echo ""

# ==========================================
# Check Go Version
# ==========================================
echo -e "${BLUE}Checking Go version...${NC}"

if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    REQUIRED_VERSION="1.22"

    if [ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" = "$REQUIRED_VERSION" ]; then
        echo -e "${GREEN}✅${NC} Go version: $GO_VERSION (>= $REQUIRED_VERSION required)"
    else
        echo -e "${RED}❌${NC} Go version: $GO_VERSION (>= $REQUIRED_VERSION required)"
        ERROR_COUNT=$((ERROR_COUNT + 1))
    fi
else
    echo -e "${RED}❌${NC} Go not found"
    ERROR_COUNT=$((ERROR_COUNT + 1))
fi

echo ""

# ==========================================
# Check Podman Status
# ==========================================
echo -e "${BLUE}Checking Podman status...${NC}"

if command -v podman &> /dev/null; then
    if podman ps &> /dev/null; then
        echo -e "${GREEN}✅${NC} Podman: running"

        # Check for test containers
        TEST_CONTAINERS=$(podman ps -a --filter name=test- --format "{{.Names}}" | wc -l)
        if [ "$TEST_CONTAINERS" -gt 0 ]; then
            echo -e "${YELLOW}⚠️${NC} Found $TEST_CONTAINERS existing test containers"
            podman ps -a --filter name=test- --format "  - {{.Names}} ({{.Status}})"
            echo -e "${YELLOW}   Run 'make force-cleanup-all-test-containers' to clean up${NC}"
            WARNING_COUNT=$((WARNING_COUNT + 1))
        fi
    else
        echo -e "${RED}❌${NC} Podman: not running or not accessible"
        echo -e "${YELLOW}   On macOS: podman machine start${NC}"
        ERROR_COUNT=$((ERROR_COUNT + 1))
    fi
fi

echo ""

# ==========================================
# Check Kind Clusters
# ==========================================
echo -e "${BLUE}Checking Kind clusters...${NC}"

if command -v kind &> /dev/null; then
    if kind get clusters 2>/dev/null | grep -q kubernaut-test; then
        echo -e "${GREEN}✅${NC} Kind cluster 'kubernaut-test' exists"

        # Check if cluster is accessible
        if kubectl cluster-info --context kind-kubernaut-test &> /dev/null; then
            echo -e "${GREEN}✅${NC} Kind cluster: accessible"
        else
            echo -e "${RED}❌${NC} Kind cluster: exists but not accessible"
            ERROR_COUNT=$((ERROR_COUNT + 1))
        fi
    else
        echo -e "${YELLOW}⚠️${NC} Kind cluster 'kubernaut-test' does not exist"
        echo -e "${YELLOW}   Run 'make create-test-cluster' to create it${NC}"
        WARNING_COUNT=$((WARNING_COUNT + 1))
    fi
fi

echo ""

# ==========================================
# Check Envtest Binaries
# ==========================================
echo -e "${BLUE}Checking Envtest binaries...${NC}"

if command -v setup-envtest &> /dev/null; then
    echo -e "${GREEN}✅${NC} setup-envtest: installed"

    # Check if binaries are available
    ENVTEST_PATH=$(setup-envtest use -p path 2>&1)
    if [ -d "$ENVTEST_PATH" ]; then
        echo -e "${GREEN}✅${NC} Envtest binaries: available at $ENVTEST_PATH"

        # Check for required binaries
        if [ -f "$ENVTEST_PATH/kube-apiserver" ]; then
            echo -e "${GREEN}✅${NC} kube-apiserver: found"
        else
            echo -e "${RED}❌${NC} kube-apiserver: not found"
            ERROR_COUNT=$((ERROR_COUNT + 1))
        fi

        if [ -f "$ENVTEST_PATH/etcd" ]; then
            echo -e "${GREEN}✅${NC} etcd: found"
        else
            echo -e "${RED}❌${NC} etcd: not found"
            ERROR_COUNT=$((ERROR_COUNT + 1))
        fi
    else
        echo -e "${RED}❌${NC} Envtest binaries: not found"
        echo -e "${YELLOW}   Run 'make install-envtest' to install${NC}"
        ERROR_COUNT=$((ERROR_COUNT + 1))
    fi
else
    echo -e "${YELLOW}⚠️${NC} setup-envtest: not installed"
    echo -e "${YELLOW}   Run 'make install-envtest' to install${NC}"
    WARNING_COUNT=$((WARNING_COUNT + 1))
fi

echo ""

# ==========================================
# Check Running Services (if applicable)
# ==========================================
echo -e "${BLUE}Checking running services...${NC}"

# Check PostgreSQL (Remediation Processor)
if podman ps --filter name=test-postgres-remediation --format "{{.Names}}" | grep -q test-postgres-remediation; then
    echo -e "${GREEN}✅${NC} PostgreSQL (remediation): running"
    if check_service "PostgreSQL" "localhost" "5433"; then
        # Check pgvector extension
        if podman exec test-postgres-remediation psql -U remediation_user -d remediation_test -c "SELECT * FROM pg_extension WHERE extname='vector'" 2>/dev/null | grep -q vector; then
            echo -e "${GREEN}✅${NC} pgvector extension: installed"
        else
            echo -e "${RED}❌${NC} pgvector extension: not installed"
            ERROR_COUNT=$((ERROR_COUNT + 1))
        fi
    fi
else
    echo -e "${YELLOW}⚠️${NC} PostgreSQL (remediation): not running"
    echo -e "${YELLOW}   Run 'make bootstrap-envtest-podman-remediationprocessor' to start${NC}"
fi

# Check Redis (Remediation Processor)
if podman ps --filter name=test-redis-remediation --format "{{.Names}}" | grep -q test-redis-remediation; then
    echo -e "${GREEN}✅${NC} Redis (remediation): running"
    check_service "Redis" "localhost" "6380"
else
    echo -e "${YELLOW}⚠️${NC} Redis (remediation): not running"
    echo -e "${YELLOW}   Run 'make bootstrap-envtest-podman-remediationprocessor' to start${NC}"
fi

echo ""

# ==========================================
# Check Database Schema
# ==========================================
echo -e "${BLUE}Checking database schema...${NC}"

if [ -f "db/migrations/remediation_audit_schema.sql" ]; then
    echo -e "${GREEN}✅${NC} remediation_audit schema file: exists"
else
    echo -e "${RED}❌${NC} remediation_audit schema file: missing"
    echo -e "${YELLOW}   Expected at: db/migrations/remediation_audit_schema.sql${NC}"
    ERROR_COUNT=$((ERROR_COUNT + 1))
fi

echo ""

# ==========================================
# Check CRD Definitions
# ==========================================
echo -e "${BLUE}Checking CRD definitions...${NC}"

check_crd() {
    local crd_path=$1
    local crd_name=$2

    if [ -f "$crd_path" ]; then
        echo -e "${GREEN}✅${NC} $crd_name: found"
    else
        echo -e "${RED}❌${NC} $crd_name: not found at $crd_path"
        ERROR_COUNT=$((ERROR_COUNT + 1))
    fi
}

check_crd "api/remediation/v1alpha1/remediationrequest_types.go" "RemediationRequest"
check_crd "api/remediationprocessing/v1alpha1/remediationprocessing_types.go" "RemediationProcessing"
check_crd "api/workflowexecution/v1alpha1/workflowexecution_types.go" "WorkflowExecution"
echo ""

# ==========================================
# Check Test Fixtures
# ==========================================
echo -e "${BLUE}Checking test fixtures...${NC}"

echo ""

# ==========================================
# Check Configuration Files
# ==========================================
echo -e "${BLUE}Checking configuration files...${NC}"

if [ -f "hack/kind-config.yaml" ]; then
    echo -e "${GREEN}✅${NC} Kind configuration: found"
else
    echo -e "${YELLOW}⚠️${NC} Kind configuration: not found"
    echo -e "${YELLOW}   Expected at: hack/kind-config.yaml${NC}"
    WARNING_COUNT=$((WARNING_COUNT + 1))
fi

if [ -f "config/integration-test.yaml" ]; then
    echo -e "${GREEN}✅${NC} Integration test config: found"
else
    echo -e "${YELLOW}⚠️${NC} Integration test config: not found"
    echo -e "${YELLOW}   Expected at: config/integration-test.yaml${NC}"
    WARNING_COUNT=$((WARNING_COUNT + 1))
fi

echo ""

# ==========================================
# Summary
# ==========================================
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Summary${NC}"
echo -e "${BLUE}========================================${NC}"

if [ $ERROR_COUNT -eq 0 ] && [ $WARNING_COUNT -eq 0 ]; then
    echo -e "${GREEN}✅ All checks passed! Infrastructure is ready for testing.${NC}"
    exit 0
elif [ $ERROR_COUNT -eq 0 ]; then
    echo -e "${YELLOW}⚠️  $WARNING_COUNT warning(s) found. Infrastructure is usable but not optimal.${NC}"
    exit 0
else
    echo -e "${RED}❌ $ERROR_COUNT error(s) and $WARNING_COUNT warning(s) found.${NC}"
    echo ""
    echo -e "${YELLOW}Next steps:${NC}"
    if ! command -v podman &> /dev/null; then
        echo -e "  - Install Podman: ${BLUE}brew install podman${NC} (macOS) or ${BLUE}sudo apt-get install podman${NC} (Linux)"
    fi
    if ! command -v kind &> /dev/null; then
        echo -e "  - Install Kind: ${BLUE}brew install kind${NC} (macOS) or download from https://kind.sigs.k8s.io/"
    fi
    if ! command -v setup-envtest &> /dev/null; then
        echo -e "  - Install Envtest: ${BLUE}make install-envtest${NC}"
    fi
    if ! kind get clusters 2>/dev/null | grep -q kubernaut-test; then
        echo -e "  - Create Kind cluster: ${BLUE}make create-test-cluster${NC}"
    fi
    echo ""
    echo -e "For complete setup, run: ${BLUE}make bootstrap-dev${NC}"
    exit 1
fi

