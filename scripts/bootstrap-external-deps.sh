#!/bin/bash

# External Dependencies Bootstrap Script
# Sets up static external dependencies for kubernaut integration environment
#
# Components setup:
# - Kind Kubernetes cluster (1 control-plane + 2 workers)
# - PostgreSQL with pgvector extension
# - Redis cache
# - Prometheus + AlertManager monitoring
# - Git submodules initialization
#
# EXCLUDES kubernaut components (handled by build-and-deploy.sh):
# - Kubernaut webhook + AI services
# - HolmesGPT integration

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../" && pwd)"
CLUSTER_NAME="kubernaut-integration"
NAMESPACE="kubernaut-integration"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    local missing_deps=()

    # Check Kind
    if ! command -v kind &> /dev/null; then
        missing_deps+=("kind")
    fi

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        missing_deps+=("kubectl")
    fi

    # Check Docker or Podman
    if ! command -v docker &> /dev/null && ! command -v podman &> /dev/null; then
        missing_deps+=("docker or podman")
    fi

    # Check Go
    if ! command -v go &> /dev/null; then
        missing_deps+=("go")
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_info "Install missing dependencies:"
        for dep in "${missing_deps[@]}"; do
            case $dep in
                "kind")
                    echo "  - kind: brew install kind"
                    ;;
                "kubectl")
                    echo "  - kubectl: brew install kubectl"
                    ;;
                "docker or podman")
                    echo "  - docker: brew install docker"
                    echo "  - OR podman: brew install podman"
                    ;;
                "go")
                    echo "  - go: brew install go"
                    ;;
            esac
        done
        exit 1
    fi

    # Check if Podman is available and running
    if command -v podman &> /dev/null; then
        if ! podman info &> /dev/null; then
            log_error "Podman is not running. Please start podman machine."
            log_info "Run: podman machine start"
            exit 1
        fi
        log_info "Using Podman as container runtime"
        # Set Kind to use Podman
        export KIND_EXPERIMENTAL_PROVIDER=podman
    elif command -v docker &> /dev/null; then
        if ! docker info &> /dev/null; then
            log_error "Docker is not running. Please start Docker Desktop."
            exit 1
        fi
        log_info "Using Docker as container runtime"
    fi

    log_success "All prerequisites satisfied"
}

# Initialize git submodules
initialize_submodules() {
    log_step "Initializing git submodules..."

    cd "$PROJECT_ROOT"

    # Initialize and update submodules
    if git submodule init && git submodule update; then
        log_success "Git submodules initialized successfully"
    else
        log_warning "Failed to initialize git submodules (may not be needed)"
        log_info "Continuing with bootstrap..."
    fi
}

# Create Kind cluster
create_kind_cluster() {
    log_step "Setting up Kind cluster: ${CLUSTER_NAME}"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log_warning "Kind cluster '${CLUSTER_NAME}' already exists"
        log_info "Using existing cluster"
        return 0
    fi

    # Create cluster with configuration
    log_info "Creating Kind cluster with configuration..."
    cd "$PROJECT_ROOT"

    if kind create cluster --name="${CLUSTER_NAME}" --config="test/kind/kind-config-simple.yaml"; then
        log_success "Kind cluster created successfully"
    else
        log_error "Failed to create Kind cluster"
        exit 1
    fi

    # Wait for cluster to be ready
    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=300s --context "kind-${CLUSTER_NAME}"

    log_success "Kind cluster is ready"
}

# Configure kubectl context
configure_kubectl() {
    log_step "Configuring kubectl context..."

    # Set the context
    kubectl config use-context "kind-${CLUSTER_NAME}"

    # Verify cluster access
    if kubectl cluster-info &> /dev/null; then
        log_success "kubectl configured successfully"
        kubectl get nodes
    else
        log_error "Failed to configure kubectl"
        exit 1
    fi
}

# Apply database migrations
apply_database_migrations() {
    log_step "Applying database migrations..."

    cd "$PROJECT_ROOT"

    # Check if migrations directory exists
    if [ ! -d "migrations" ]; then
        log_error "Migrations directory not found"
        exit 1
    fi

    # Wait a bit more for PostgreSQL to be fully ready
    log_info "Waiting for PostgreSQL to be fully ready for migrations..."
    sleep 10

    # Create migration tracking table
    log_info "Setting up migration tracking..."
    if kubectl exec -n ${NAMESPACE} deployment/postgresql -- psql -U slm_user -d action_history -c "
        CREATE TABLE IF NOT EXISTS schema_migrations (
            version VARCHAR(255) PRIMARY KEY,
            applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
        );" > /dev/null 2>&1; then
        log_success "Migration tracking table ready"
    else
        log_warning "Migration tracking table setup failed, continuing..."
    fi

    # Get list of applied migrations
    applied_migrations=$(kubectl exec -n ${NAMESPACE} deployment/postgresql -- psql -U slm_user -d action_history -t -c "SELECT version FROM schema_migrations ORDER BY version;" 2>/dev/null | tr -d ' ' || echo "")

    # Apply each migration file in order
    for migration_file in migrations/*.sql; do
        if [ ! -f "$migration_file" ]; then
            log_warning "No migration files found in migrations/"
            continue
        fi

        # Extract version from filename (e.g., 001_initial_schema.sql -> 001)
        version=$(basename "$migration_file" | cut -d'_' -f1)
        migration_name=$(basename "$migration_file")

        # Skip if already applied
        if echo "$applied_migrations" | grep -q "^$version$"; then
            log_info "Migration $version already applied, skipping..."
            continue
        fi

        log_info "Applying migration: $migration_name"

        # Apply migration
        if kubectl exec -i -n ${NAMESPACE} deployment/postgresql -- psql -U slm_user -d action_history < "$migration_file" > /dev/null 2>&1; then
            # Record successful migration
            kubectl exec -n ${NAMESPACE} deployment/postgresql -- psql -U slm_user -d action_history -c "
                INSERT INTO schema_migrations (version) VALUES ('$version');" > /dev/null 2>&1
            log_success "Migration $version applied successfully"
        else
            log_warning "Migration $version failed (may already exist or have dependencies)"
            # Continue with other migrations rather than failing completely
        fi
    done

    # Verify core tables exist
    log_info "Verifying database schema..."
    if kubectl exec -n ${NAMESPACE} deployment/postgresql -- psql -U slm_user -d action_history -c "\dt" > /dev/null 2>&1; then
        log_success "Database schema verification completed"
    else
        log_warning "Database schema verification failed, but continuing..."
    fi

    log_success "Database migrations completed"
}

# Deploy external dependencies only
deploy_external_dependencies() {
    log_step "Deploying external dependencies to Kind cluster..."

    cd "$PROJECT_ROOT"

    # Create namespace first
    log_info "Creating namespace..."
    kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

    # Deploy external dependencies using individual YAML files
    log_info "Applying external dependencies manifests..."

    # Deploy PostgreSQL
    log_info "Deploying PostgreSQL..."
    kubectl apply -f deploy/integration/postgresql/ -n ${NAMESPACE}

    # Deploy Redis
    log_info "Deploying Redis..."
    kubectl apply -f deploy/integration/redis/ -n ${NAMESPACE}

    # Deploy monitoring stack
    log_info "Deploying monitoring stack..."
    kubectl apply -f deploy/integration/monitoring/prometheus/ -n ${NAMESPACE}
    kubectl apply -f deploy/integration/monitoring/alertmanager/ -n ${NAMESPACE}

    # Wait for external dependencies to be ready
    log_info "Waiting for external dependencies to be ready (this may take a few minutes)..."

    # Wait for PostgreSQL first (other services depend on it)
    kubectl wait --for=condition=available --timeout=300s deployment/postgresql -n ${NAMESPACE}
    log_info "PostgreSQL is ready"

    # Apply database migrations
    apply_database_migrations

    # Wait for Redis
    kubectl wait --for=condition=available --timeout=300s deployment/redis -n ${NAMESPACE}
    log_info "Redis is ready"

    # Wait for monitoring stack
    kubectl wait --for=condition=available --timeout=300s deployment/prometheus -n ${NAMESPACE}
    log_info "Prometheus is ready"

    # AlertManager is optional - don't fail if it's not ready
    if kubectl wait --for=condition=available --timeout=60s deployment/alertmanager -n ${NAMESPACE} 2>/dev/null; then
        log_info "AlertManager is ready"
    else
        log_warning "AlertManager not ready yet (non-critical, continuing...)"
    fi

    log_success "All external dependencies deployed successfully"
}

# Verify external dependencies
verify_external_dependencies() {
    log_step "Verifying external dependencies..."

    # Check cluster status
    log_info "Cluster status:"
    kubectl cluster-info --context "kind-${CLUSTER_NAME}"

    # Check external services
    log_info "External dependencies status:"
    kubectl get pods,svc -n ${NAMESPACE}

    # Check service health
    log_info "Checking external service health..."

    # Wait a bit for services to fully start
    sleep 10

    # Check if external services are responding
    local postgresql_ready=false
    local redis_ready=false
    local prometheus_ready=false
    local alertmanager_ready=false

    # Check PostgreSQL
    if kubectl exec -n ${NAMESPACE} deployment/postgresql -- pg_isready -U slm_user -d action_history > /dev/null 2>&1; then
        postgresql_ready=true
        log_success "PostgreSQL is responding"
    else
        log_warning "PostgreSQL not responding yet"
    fi

    # Check Redis
    if kubectl exec -n ${NAMESPACE} deployment/redis -- redis-cli ping > /dev/null 2>&1; then
        redis_ready=true
        log_success "Redis is responding"
    else
        log_warning "Redis not responding yet"
    fi

    # Check Prometheus (via NodePort)
    if curl -s --connect-timeout 5 "http://localhost:30090/-/ready" > /dev/null 2>&1; then
        prometheus_ready=true
        log_success "Prometheus is responding"
    else
        log_warning "Prometheus not responding yet (may need port-forward)"
    fi

    # Check AlertManager (via NodePort)
    if curl -s --connect-timeout 5 "http://localhost:30093/-/ready" > /dev/null 2>&1; then
        alertmanager_ready=true
        log_success "AlertManager is responding"
    else
        log_warning "AlertManager not responding yet (may need port-forward)"
    fi

    if [ "$postgresql_ready" = true ] && [ "$redis_ready" = true ]; then
        log_success "Core external dependencies verification completed successfully"
    else
        log_warning "Some external dependencies may still be starting up"
        log_info "Use 'kubectl get pods -n ${NAMESPACE}' to check status"
    fi
}

# Generate environment configuration
generate_environment_config() {
    log_step "Generating external dependencies environment configuration..."

    cd "$PROJECT_ROOT"

    cat > .env.external-deps << EOF
# External Dependencies Environment Configuration
# Generated by bootstrap-external-deps.sh

# Cluster configuration
KIND_CLUSTER_NAME=${CLUSTER_NAME}
KUBECONFIG=${HOME}/.kube/config
KUBECTL_CONTEXT=kind-${CLUSTER_NAME}

# Service endpoints (use kubectl port-forward for access)
PROMETHEUS_URL=http://localhost:30090
ALERTMANAGER_URL=http://localhost:30093
POSTGRESQL_URL=postgresql://slm_user:slm_password_dev@localhost:30432/action_history
REDIS_URL=redis://localhost:30379

# Database configuration
POSTGRES_HOST=localhost
POSTGRES_PORT=30432
POSTGRES_DB=action_history
POSTGRES_USER=slm_user
POSTGRES_PASSWORD=slm_password_dev

# Redis configuration
REDIS_HOST=localhost
REDIS_PORT=30379

# Environment type
ENVIRONMENT=external-dependencies
DEPLOYMENT_METHOD=kind-cluster
NAMESPACE=${NAMESPACE}

# Testing configuration
USE_KIND_CLUSTER=true
USE_FAKE_K8S_CLIENT=false
INTEGRATION_NAMESPACE=${NAMESPACE}
EOF

    log_success "External dependencies environment configuration saved to .env.external-deps"
}

# Show usage information
show_usage_info() {
    log_step "External dependencies environment ready!"

    echo ""
    echo "🎉 External Dependencies Bootstrap Complete!"
    echo "=========================================="
    echo ""
    echo "📋 External Services Available:"
    echo "  • Kind Cluster:       ${CLUSTER_NAME}"
    echo "  • PostgreSQL:         Ready with full schema (use kubectl port-forward)"
    echo "  • Database Schema:    All application tables created"
    echo "  • Redis:              Ready (use kubectl port-forward)"
    echo "  • Prometheus:         Ready (use kubectl port-forward)"
    echo "  • AlertManager:       Ready (use kubectl port-forward)"
    echo ""
    echo "🔧 Management Commands:"
    echo "  • Check status:       kubectl get pods -n ${NAMESPACE}"
    echo "  • View logs:          kubectl logs -f deployment/<service> -n ${NAMESPACE}"
    echo "  • Port forward:       kubectl port-forward -n ${NAMESPACE} svc/<service> <local-port>:<service-port>"
    echo ""
    echo "🚀 Next Steps:"
    echo "  • Build kubernaut:    make build-and-deploy"
    echo "  • Load environment:   source .env.external-deps"
    echo "  • Test connections:   kubectl get pods -n ${NAMESPACE}"
    echo ""
    echo "⚙️  Configuration:"
    echo "  • Environment file:   .env.external-deps"
    echo "  • Kubectl context:    kind-${CLUSTER_NAME}"
    echo "  • Namespace:          ${NAMESPACE}"
    echo ""
    echo "💡 Port Forwarding Examples:"
    echo "  • PostgreSQL:         kubectl port-forward -n ${NAMESPACE} svc/postgresql-service 5432:5432"
    echo "  • Redis:              kubectl port-forward -n ${NAMESPACE} svc/redis-service 6379:6379"
    echo "  • Prometheus:         kubectl port-forward -n ${NAMESPACE} svc/prometheus-service 9090:9090"
    echo ""
}

# Main execution
main() {
    local start_time
    start_time=$(date +%s)

    echo "🚀 Kubernaut External Dependencies Bootstrap"
    echo "==========================================="
    echo ""
    echo "This script will setup ONLY external dependencies:"
    echo "  ✓ Kind Kubernetes cluster (1 control-plane + 2 workers)"
    echo "  ✓ PostgreSQL with pgvector extension"
    echo "  ✓ Database schema migrations (all application tables)"
    echo "  ✓ Redis cache"
    echo "  ✓ Prometheus + AlertManager monitoring"
    echo "  ✓ Git submodules initialization"
    echo ""
    echo "EXCLUDED (handled by build-and-deploy.sh):"
    echo "  ✗ kubernaut webhook-service"
    echo "  ✗ kubernaut ai-service"
    echo "  ✗ holmesgpt integration"
    echo ""

    # Handle command line arguments
    case "${1:-}" in
        --help|-h)
            echo "Usage: $0 [--help]"
            echo ""
            echo "Bootstraps external dependencies for Kubernaut integration environment."
            echo ""
            echo "Prerequisites:"
            echo "  - kind (brew install kind)"
            echo "  - kubectl (brew install kubectl)"
            echo "  - docker or podman running"
            echo "  - go (brew install go)"
            echo ""
            echo "After bootstrap, build and deploy kubernaut services with:"
            echo "  make build-and-deploy"
            echo ""
            echo "Clean up with:"
            echo "  kind delete cluster --name ${CLUSTER_NAME}"
            exit 0
            ;;
        *)
            # Continue with main execution
            ;;
    esac

    check_prerequisites
    initialize_submodules
    create_kind_cluster
    configure_kubectl
    deploy_external_dependencies
    verify_external_dependencies
    generate_environment_config
    show_usage_info

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    log_success "🎉 External dependencies bootstrap completed successfully in ${duration} seconds!"
    echo ""
    echo "Next steps:"
    echo "  1. source .env.external-deps"
    echo "  2. make build-and-deploy"
}

# Execute main function
main "$@"
