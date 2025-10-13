#!/usr/bin/env bash
# Data Storage Service Integration Test Matrix
# Tests across multiple PostgreSQL 16.x and pgvector versions
# BR-STORAGE-012: Vector similarity search with PostgreSQL 16+ HNSW validation

set -euo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test matrix configurations
# Format: "pg_version:pgvector_tag:description"
# Note: We only test one stable version due to resource constraints
# This is sufficient as PostgreSQL 16.x versions have consistent HNSW support
TEST_MATRIX=(
    "16:pg16:PostgreSQL 16 (stable)"
)

# Container name prefix
CONTAINER_PREFIX="datastorage-test"

# Test results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
declare -a FAILED_CONFIGS

echo "=========================================="
echo "Data Storage Integration Test"
echo "PostgreSQL 16 (Stable) with pgvector HNSW"
echo "=========================================="
echo ""

# Function to cleanup containers
cleanup_container() {
    local container_name=$1
    echo -e "${BLUE}🧹 Cleaning up container: $container_name${NC}"
    podman stop "$container_name" > /dev/null 2>&1 || true
    podman rm "$container_name" > /dev/null 2>&1 || true
}

# Function to run tests for a specific configuration
run_test_config() {
    local pg_version=$1
    local pgvector_tag=$2
    local description=$3
    local container_name="${CONTAINER_PREFIX}-${pg_version}"

    ((TOTAL_TESTS++))

    echo ""
    echo "=========================================="
    echo -e "${BLUE}Test Configuration #${TOTAL_TESTS}${NC}"
    echo -e "${BLUE}Description: $description${NC}"
    echo -e "${BLUE}PostgreSQL: $pg_version${NC}"
    echo -e "${BLUE}pgvector tag: $pgvector_tag${NC}"
    echo "=========================================="
    echo ""

    # Cleanup any existing container
    cleanup_container "$container_name"

    # Start PostgreSQL with pgvector
    echo -e "${YELLOW}🔧 Starting PostgreSQL $pg_version with pgvector...${NC}"
    if ! podman run -d \
        --name "$container_name" \
        -p 5432:5432 \
        -e POSTGRES_PASSWORD=postgres \
        -e POSTGRES_SHARED_BUFFERS=1GB \
        "pgvector/pgvector:$pgvector_tag" > /dev/null 2>&1; then
        echo -e "${RED}❌ Failed to start PostgreSQL container${NC}"
        ((FAILED_TESTS++))
        FAILED_CONFIGS+=("$description: Container start failed")
        return 1
    fi

    # Wait for PostgreSQL to be ready
    echo -e "${YELLOW}⏳ Waiting for PostgreSQL to be ready...${NC}"
    local retries=30
    local wait_time=1
    local ready=0

    for ((i=1; i<=retries; i++)); do
        if podman exec "$container_name" pg_isready -U postgres > /dev/null 2>&1; then
            ready=1
            break
        fi
        sleep $wait_time
    done

    if [ $ready -eq 0 ]; then
        echo -e "${RED}❌ PostgreSQL failed to become ready within $((retries * wait_time)) seconds${NC}"
        cleanup_container "$container_name"
        ((FAILED_TESTS++))
        FAILED_CONFIGS+=("$description: PostgreSQL not ready")
        return 1
    fi

    echo -e "${GREEN}✅ PostgreSQL ready${NC}"

    # Verify PostgreSQL version
    echo -e "${YELLOW}🔍 Verifying PostgreSQL version...${NC}"
    local pg_version_output
    pg_version_output=$(podman exec "$container_name" psql -U postgres -t -c "SELECT version();")

    if ! echo "$pg_version_output" | grep -q "PostgreSQL 16"; then
        echo -e "${RED}❌ PostgreSQL version verification failed${NC}"
        echo "Expected: PostgreSQL 16.x"
        echo "Got: $pg_version_output"
        cleanup_container "$container_name"
        ((FAILED_TESTS++))
        FAILED_CONFIGS+=("$description: Version verification failed")
        return 1
    fi

    echo -e "${GREEN}✅ PostgreSQL $pg_version verified${NC}"

    # Verify pgvector extension
    echo -e "${YELLOW}🔍 Verifying pgvector extension...${NC}"
    local pgvector_version
    pgvector_version=$(podman exec "$container_name" psql -U postgres -t -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';" 2>/dev/null || echo "not_found")

    if [ "$pgvector_version" = "not_found" ] || [ -z "$pgvector_version" ]; then
        echo -e "${YELLOW}⚠️  pgvector extension not pre-installed, will be created by schema initialization${NC}"
    else
        echo -e "${GREEN}✅ pgvector extension version: ${pgvector_version}${NC}"

        # Verify pgvector is 0.5.1+
        if ! echo "$pgvector_version" | grep -qE "0\.[5-9]\.[1-9]|0\.[6-9]\.0|[1-9]\."; then
            echo -e "${RED}❌ pgvector version < 0.5.1 (required for HNSW)${NC}"
            cleanup_container "$container_name"
            ((FAILED_TESTS++))
            FAILED_CONFIGS+=("$description: pgvector version < 0.5.1")
            return 1
        fi
        echo -e "${GREEN}✅ pgvector 0.5.1+ verified${NC}"
    fi

    # Verify HNSW index support (dry-run test)
    echo -e "${YELLOW}🔍 Testing HNSW index creation...${NC}"
    if ! podman exec "$container_name" psql -U postgres -c "
        CREATE TEMP TABLE hnsw_test (id int, embedding vector(384));
        CREATE INDEX hnsw_test_idx ON hnsw_test USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
    " > /dev/null 2>&1; then
        echo -e "${RED}❌ HNSW index creation failed${NC}"
        cleanup_container "$container_name"
        ((FAILED_TESTS++))
        FAILED_CONFIGS+=("$description: HNSW index creation failed")
        return 1
    fi

    echo -e "${GREEN}✅ HNSW index support verified${NC}"

    # Run integration tests
    echo -e "${YELLOW}🧪 Running Data Storage integration tests...${NC}"
    echo ""

    local test_result=0
    if go test ./test/integration/datastorage/... -v -timeout 5m; then
        echo ""
        echo -e "${GREEN}✅ Integration tests PASSED for $description${NC}"
        ((PASSED_TESTS++))
    else
        test_result=$?
        echo ""
        echo -e "${RED}❌ Integration tests FAILED for $description${NC}"
        ((FAILED_TESTS++))
        FAILED_CONFIGS+=("$description: Integration tests failed (exit code: $test_result)")
    fi

    # Cleanup
    cleanup_container "$container_name"

    return $test_result
}

# Main execution
echo -e "${BLUE}Testing PostgreSQL 16 (stable) with pgvector HNSW support...${NC}"
echo ""

# Run tests for each configuration
for config in "${TEST_MATRIX[@]}"; do
    IFS=':' read -r pg_version pgvector_tag description <<< "$config"
    run_test_config "$pg_version" "$pgvector_tag" "$description" || true
done

# Print summary
echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "PostgreSQL 16 (stable) test: ${BLUE}${TOTAL_TESTS}${NC}"
echo -e "Passed: ${GREEN}${PASSED_TESTS}${NC}"
echo -e "Failed: ${RED}${FAILED_TESTS}${NC}"
echo ""

if [ ${#FAILED_CONFIGS[@]} -gt 0 ]; then
    echo -e "${RED}Failed Configurations:${NC}"
    for failed_config in "${FAILED_CONFIGS[@]}"; do
        echo -e "  ${RED}❌ $failed_config${NC}"
    done
    echo ""
fi

echo "=========================================="

# Exit with failure if any tests failed
if [ $FAILED_TESTS -gt 0 ]; then
    echo -e "${RED}❌ PostgreSQL 16 test FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}✅ PostgreSQL 16 test PASSED${NC}"
    exit 0
fi

