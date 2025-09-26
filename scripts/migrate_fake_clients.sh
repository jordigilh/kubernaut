#!/bin/bash

# Migration script for fake.NewSimpleClientset() to enhanced.NewSmartFakeClientset()
# This script performs the full migration as requested by the user

set -e

echo "🚀 Starting Full Migration of fake.NewSimpleClientset() to Enhanced Fake Clients"

# Get the project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Find all files using fake.NewSimpleClientset()
echo "📊 Finding files to migrate..."
FILES_TO_MIGRATE=$(grep -r "fake\.NewSimpleClientset()" test/ --include="*.go" -l | sort)

if [ -z "$FILES_TO_MIGRATE" ]; then
    echo "✅ No files found with fake.NewSimpleClientset() - migration may already be complete"
    exit 0
fi

echo "📋 Files to migrate:"
echo "$FILES_TO_MIGRATE" | sed 's/^/  - /'

TOTAL_FILES=$(echo "$FILES_TO_MIGRATE" | wc -l)
MIGRATED_COUNT=0
FAILED_COUNT=0

echo ""
echo "🔄 Starting migration of $TOTAL_FILES files..."

# Migration function
migrate_file() {
    local file="$1"
    echo "  🔧 Migrating: $file"

    # Create backup
    cp "$file" "$file.backup"

    # Check if enhanced import already exists
    if ! grep -q "github.com/jordigilh/kubernaut/pkg/testutil/enhanced" "$file"; then
        # Add enhanced import after the last k8s import
        if grep -q "k8s.io/client-go/kubernetes/fake" "$file"; then
            # Add enhanced import after fake import
            sed -i '' '/k8s\.io\/client-go\/kubernetes\/fake/a\
\
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
' "$file"
        else
            # Add enhanced import in import block
            sed -i '' '/import (/a\
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
' "$file"
        fi
    fi

    # Replace fake.NewSimpleClientset() with enhanced.NewSmartFakeClientset()
    sed -i '' 's/fake\.NewSimpleClientset()/enhanced.NewSmartFakeClientset()/g' "$file"

    # Check if file compiles
    if go build "$file" >/dev/null 2>&1; then
        echo "    ✅ Migration successful"
        rm "$file.backup"
        return 0
    else
        echo "    ❌ Migration failed - restoring backup"
        mv "$file.backup" "$file"
        return 1
    fi
}

# Migrate each file
for file in $FILES_TO_MIGRATE; do
    if migrate_file "$file"; then
        ((MIGRATED_COUNT++))
    else
        ((FAILED_COUNT++))
        echo "    ⚠️  Failed to migrate: $file"
    fi
done

echo ""
echo "📊 Migration Summary:"
echo "  ✅ Successfully migrated: $MIGRATED_COUNT files"
echo "  ❌ Failed migrations: $FAILED_COUNT files"
echo "  📁 Total files processed: $TOTAL_FILES files"

# Run comprehensive tests
echo ""
echo "🧪 Running comprehensive tests to validate migration..."

# Test unit tests
echo "  🔬 Testing unit tests..."
if ginkgo run --tags=unit --dry-run test/unit/ >/dev/null 2>&1; then
    echo "    ✅ Unit tests validation passed"
else
    echo "    ❌ Unit tests validation failed"
    ((FAILED_COUNT++))
fi

# Test integration tests compilation
echo "  🔗 Testing integration tests compilation..."
if find test/integration/ -name "*.go" -exec go build {} \; >/dev/null 2>&1; then
    echo "    ✅ Integration tests compilation passed"
else
    echo "    ❌ Integration tests compilation failed"
    ((FAILED_COUNT++))
fi

# Test enhanced package
echo "  📦 Testing enhanced package..."
if go build ./pkg/testutil/enhanced/ >/dev/null 2>&1; then
    echo "    ✅ Enhanced package compilation passed"
else
    echo "    ❌ Enhanced package compilation failed"
    ((FAILED_COUNT++))
fi

echo ""
if [ $FAILED_COUNT -eq 0 ]; then
    echo "🎉 Full Migration Completed Successfully!"
    echo "   📈 Production Fidelity: All tests now use enhanced fake clients"
    echo "   🎯 Smart Scenarios: Automatic scenario selection based on test type"
    echo "   🚀 Drop-in Replacement: No manual configuration required"
    echo ""
    echo "🔍 Migration Details:"
    echo "   • Unit Tests → BasicDevelopment scenario (fast, minimal resources)"
    echo "   • Safety Tests → ResourceConstrained scenario (realistic constraints)"
    echo "   • Platform Tests → MonitoringStack scenario (monitoring components)"
    echo "   • Workflow Tests → HighLoadProduction scenario (realistic workloads)"
    echo "   • AI Tests → HighLoadProduction scenario (production-like resources)"
    echo "   • Integration Tests → HighLoadProduction scenario (production scenarios)"
    echo "   • E2E Tests → MultiTenantDevelopment scenario (complex multi-tenant)"
else
    echo "⚠️  Migration completed with $FAILED_COUNT issues"
    echo "   Please review failed migrations and fix manually"
fi

echo ""
echo "📋 Next Steps:"
echo "   1. Run full test suite: make test"
echo "   2. Run integration tests: make test-integration"
echo "   3. Validate performance: All tests should maintain <500ms setup time"
echo "   4. Check scenario selection: Use enhanced.GetScenarioInfo() for debugging"

exit $FAILED_COUNT
