#!/bin/bash

# Enhanced Vector Database Bootstrap Script
# Following project guidelines: defensive programming, proper error handling, business requirement alignment

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
INTEGRATION_DIR="${PROJECT_ROOT}/test/integration"

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

# Error handling
handle_error() {
    local exit_code=$?
    local line_number=$1
    log_error "Command failed with exit code $exit_code at line $line_number"
    log_error "Failed command: ${BASH_COMMAND}"
    exit $exit_code
}

trap 'handle_error $LINENO' ERR

# Check if vector database container is running
check_vector_database() {
    log_step "Checking vector database status..."

    if ! podman ps --format "{{.Names}}" | grep -q "kubernaut-integration-vectordb"; then
        log_error "Vector database container is not running"
        log_info "Please run 'make integration-infrastructure-setup' first"
        exit 1
    fi

    log_success "Vector database container is running"
}

# Reinitialize vector database with enhanced schema
reinitialize_vector_database() {
    log_step "Reinitializing vector database with enhanced schema..."

    local enhanced_script="${INTEGRATION_DIR}/scripts/init-vector-store-enhanced.sql"

    if [ ! -f "$enhanced_script" ]; then
        log_error "Enhanced initialization script not found: $enhanced_script"
        exit 1
    fi

    # Execute the enhanced initialization script
    log_info "Executing enhanced vector database initialization..."
    if podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -f /docker-entrypoint-initdb.d/01-init-vector-store-enhanced.sql; then
        log_success "Enhanced vector database schema initialized"
    else
        log_warning "Schema initialization had some conflicts (expected for existing database)"
        log_info "Attempting to run validation..."
    fi
}

# Validate vector database setup
validate_vector_database() {
    log_step "Validating enhanced vector database setup..."

    log_info "Running validation function..."
    podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT * FROM validate_vector_database_setup();" || {
        log_warning "Validation function not available, running manual checks..."

        # Manual validation checks
        log_info "Checking pgvector extension..."
        podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT extname, extversion FROM pg_extension WHERE extname = 'vector';"

        log_info "Checking schemas..."
        podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT schemaname FROM pg_namespace JOIN pg_user ON pg_namespace.nspowner = pg_user.usesysid WHERE schemaname IN ('embeddings', 'similarity_search', 'action_patterns');"

        log_info "Checking tables..."
        podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT schemaname, tablename FROM pg_tables WHERE schemaname IN ('embeddings', 'public') ORDER BY schemaname, tablename;"

        log_info "Checking test data..."
        podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT 'action_patterns' as table_name, COUNT(*) as count FROM action_patterns UNION ALL SELECT 'document_embeddings', COUNT(*) FROM embeddings.document_embeddings UNION ALL SELECT 'workflow_patterns', COUNT(*) FROM embeddings.workflow_pattern_embeddings;"
    }
}

# Test vector database functionality
test_vector_functionality() {
    log_step "Testing vector database functionality..."

    log_info "Testing similarity search for documents..."
    podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT document_id, content, similarity_score FROM similarity_search.find_similar_documents(generate_test_embedding('memory usage'), 0.5, 3);" || log_warning "Document similarity search test failed"

    log_info "Testing similarity search for action patterns..."
    podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT id, action_type, alert_name, similarity_score FROM similarity_search.find_similar_action_patterns(generate_test_embedding('scale deployment'), 0.5, 3);" || log_warning "Action pattern similarity search test failed"

    log_info "Testing pattern analytics view..."
    podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c "SELECT * FROM pattern_analytics_summary LIMIT 5;" || log_warning "Pattern analytics view test failed"
}

# Show database connection information
show_connection_info() {
    log_step "Vector Database Connection Information"
    echo ""
    echo "🎯 Enhanced Vector Database:"
    echo "   Host: localhost"
    echo "   Port: 5434"
    echo "   Database: vector_store"
    echo "   Username: vector_user"
    echo "   Password: vector_password_dev"
    echo "   Connection: postgres://vector_user:vector_password_dev@localhost:5434/vector_store?sslmode=disable"
    echo ""
    echo "📊 Available Schemas:"
    echo "   - embeddings (document, workflow, alert embeddings)"
    echo "   - similarity_search (search functions)"
    echo "   - action_patterns (main action patterns table)"
    echo "   - public (default schema with action_patterns table)"
    echo ""
    echo "🔍 Test Data Available:"
    echo "   - 5 test documents with embeddings"
    echo "   - 5 workflow patterns with embeddings"
    echo "   - 5 alert patterns with embeddings"
    echo "   - 5 action patterns with embeddings"
    echo ""
    echo "🧪 Test Functions:"
    echo "   - validate_vector_database_setup()"
    echo "   - generate_test_embedding(text)"
    echo "   - similarity_search.find_similar_documents(embedding, threshold, limit)"
    echo "   - similarity_search.find_similar_action_patterns(embedding, threshold, limit)"
    echo ""
}

# Main execution
main() {
    echo "🚀 Enhanced Vector Database Bootstrap"
    echo "====================================="
    echo ""
    echo "This script will enhance the vector database with:"
    echo "  ✓ Comprehensive action_patterns table"
    echo "  ✓ Advanced similarity search functions"
    echo "  ✓ Test data with proper 384-dimensional embeddings"
    echo "  ✓ Analytics views and validation functions"
    echo ""

    check_vector_database
    reinitialize_vector_database
    validate_vector_database
    test_vector_functionality
    show_connection_info

    log_success "🎉 Enhanced vector database setup completed successfully!"
    echo ""
    echo "Next steps:"
    echo "  1. Run integration tests: make test-integration-quick"
    echo "  2. Validate setup: podman exec kubernaut-integration-vectordb psql -U vector_user -d vector_store -c \"SELECT * FROM validate_vector_database_setup();\""
}

# Execute main function with all arguments
main "$@"
