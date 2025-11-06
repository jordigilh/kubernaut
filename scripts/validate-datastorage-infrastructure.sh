#!/usr/bin/env bash
set -euo pipefail

echo "🔍 Data Storage Service - Pre-Implementation Validation"
echo "======================================================"
echo ""

EXIT_CODE=0

# Check 1: PostgreSQL availability
echo "Check 1: PostgreSQL availability..."
if podman ps 2>/dev/null | grep -q "datastorage-postgres"; then
    echo "✅ PostgreSQL container is running"
else
    echo "❌ PostgreSQL container is NOT running"
    echo "   Fix: podman run -d --name datastorage-postgres -p 5432:5432 \\"
    echo "        -e POSTGRESQL_USER=db_user -e POSTGRESQL_PASSWORD=test \\"
    echo "        -e POSTGRESQL_DATABASE=action_history -e POSTGRESQL_ADMIN_PASSWORD=test \\"
    echo "        registry.redhat.io/rhel9/postgresql-16:latest"
    EXIT_CODE=1
fi

# Check 2: Database schema exists
echo ""
echo "Check 2: Database schema validation..."
if podman exec -i datastorage-postgres psql -U postgres -d action_history -c "\dt" 2>/dev/null | grep -q resource_action_traces; then
    echo "✅ Database schema exists (resource_action_traces table found)"
else
    echo "❌ Database schema NOT found"
    echo "   Fix: Apply migrations using: podman exec -i datastorage-postgres psql -U postgres -d action_history < migrations/001_initial_schema.sql"
    EXIT_CODE=1
fi

# Check 3: Required tables exist
echo ""
echo "Check 3: Required tables validation..."
REQUIRED_TABLES=("resource_action_traces" "action_histories" "resource_references")
for table in "${REQUIRED_TABLES[@]}"; do
    if podman exec -i datastorage-postgres psql -U postgres -d action_history -c "\dt" 2>/dev/null | grep -q "$table"; then
        echo "✅ Table '$table' exists"
    else
        echo "❌ Table '$table' NOT found"
        EXIT_CODE=1
    fi
done

# Check 4: Go dependencies
echo ""
echo "Check 4: Go dependencies verification..."
if go mod verify 2>/dev/null; then
    echo "✅ Go dependencies verified"
else
    echo "❌ Go dependencies NOT verified"
    echo "   Fix: go mod tidy"
    EXIT_CODE=1
fi

# Check 5: Required Go packages
echo ""
echo "Check 5: Required Go packages check..."
REQUIRED_PACKAGES=("github.com/lib/pq" "github.com/jmoiron/sqlx" "go.uber.org/zap")
for pkg in "${REQUIRED_PACKAGES[@]}"; do
    if go list -m "$pkg" 2>/dev/null >/dev/null; then
        echo "✅ Package '$pkg' available"
    else
        echo "❌ Package '$pkg' NOT found"
        echo "   Fix: go get $pkg"
        EXIT_CODE=1
    fi
done

# Check 6: Test infrastructure
echo ""
echo "Check 6: Test infrastructure validation..."
if [ -f "pkg/testutil/postgres_container.go" ]; then
    echo "✅ PostgreSQL test container helper exists"
else
    echo "❌ PostgreSQL test container helper NOT found"
    echo "   Fix: Create pkg/testutil/postgres_container.go"
    EXIT_CODE=1
fi

# Check 7: Podman availability (for integration tests)
echo ""
echo "Check 7: Podman availability..."
if command -v podman >/dev/null 2>&1; then
    echo "✅ Podman is installed"
    podman --version
else
    echo "❌ Podman is NOT installed"
    echo "   Fix: brew install podman (macOS) or apt-get install podman (Linux)"
    EXIT_CODE=1
fi

# Check 8: Build validation
echo ""
echo "Check 8: Data Storage Service build validation..."
if go build -o /dev/null ./cmd/datastorage 2>/dev/null; then
    echo "✅ Data Storage Service builds successfully"
else
    echo "❌ Data Storage Service build FAILED"
    echo "   Fix: go build ./cmd/datastorage"
    EXIT_CODE=1
fi

# Check 9: Lint check
echo ""
echo "Check 9: Lint validation..."
if command -v golangci-lint >/dev/null 2>&1; then
    if golangci-lint run ./pkg/datastorage/... 2>/dev/null; then
        echo "✅ No lint errors in pkg/datastorage/"
    else
        echo "❌ Lint errors found"
        echo "   Fix: golangci-lint run --fix ./pkg/datastorage/"
        EXIT_CODE=1
    fi
else
    echo "⚠️  golangci-lint not installed (optional but recommended)"
    echo "   Install: brew install golangci-lint"
fi

# Check 10: Directory structure
echo ""
echo "Check 10: Directory structure validation..."
REQUIRED_DIRS=("pkg/datastorage" "test/unit/datastorage" "test/integration/datastorage" "docs/services/stateless/data-storage")
for dir in "${REQUIRED_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        echo "✅ Directory '$dir' exists"
    else
        echo "❌ Directory '$dir' NOT found"
        echo "   Fix: mkdir -p $dir"
        EXIT_CODE=1
    fi
done

# Summary
echo ""
echo "======================================================"
if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ ALL CHECKS PASSED - Ready for Data Storage Service implementation"
    echo ""
    echo "Next steps:"
    echo "1. Review migration plan: docs/services/stateless/data-storage/implementation/API-GATEWAY-MIGRATION.md"
    echo "2. Start with Day 1 (DO-RED): Write failing tests"
    echo "3. Follow APDC-TDD workflow: Analysis → Plan → Do → Check"
else
    echo "❌ SOME CHECKS FAILED - Fix issues before starting implementation"
    echo ""
    echo "Run this script again after fixes to verify"
fi

exit $EXIT_CODE
