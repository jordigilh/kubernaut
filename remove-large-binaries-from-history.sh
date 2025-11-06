#!/bin/bash
# Script to remove large binary files from git history
# These files should never have been committed (build artifacts)

set -e  # Exit on error

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Removing large binary files from git history"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "⚠️  WARNING: This will rewrite git history!"
echo "⚠️  All commit hashes will change after this operation"
echo ""
echo "Current directory: $(pwd)"
echo ""

# Check if we're in the right directory
if [ ! -f "go.mod" ]; then
    echo "❌ Error: Not in kubernaut root directory"
    echo "Please run this script from: /Users/jgil/go/src/github.com/jordigilh/kubernaut"
    exit 1
fi

# List of large binary files to remove (build artifacts that should never be committed)
LARGE_FILES=(
    "workflowexecutor"
    "integration-webhook-server"
    "kubernaut"
    "webhook.test"
    "dynamic-toolset-server"
    "main"
    "remediationprocessor"
)

echo "Files to be removed from history:"
for file in "${LARGE_FILES[@]}"; do
    echo "  - $file"
done
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "STEP 1: Create backup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

BACKUP_DIR="/tmp/kubernaut-backup-$(date +%Y%m%d-%H%M%S)"
echo "Creating backup at: $BACKUP_DIR"
cp -r . "$BACKUP_DIR"
echo "✅ Backup created successfully"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "STEP 2: Remove files from git history using filter-branch"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Build the git rm command for all files
RM_COMMANDS=""
for file in "${LARGE_FILES[@]}"; do
    RM_COMMANDS="$RM_COMMANDS git rm -rf --cached --ignore-unmatch '$file' ; "
    RM_COMMANDS="$RM_COMMANDS git rm -rf --cached --ignore-unmatch '*/$file' ; "
    RM_COMMANDS="$RM_COMMANDS git rm -rf --cached --ignore-unmatch 'cmd/*/$file' ; "
    RM_COMMANDS="$RM_COMMANDS git rm -rf --cached --ignore-unmatch 'bin/$file' ; "
done

echo "Running git filter-branch (this may take a few minutes)..."
git filter-branch --force --index-filter \
  "$RM_COMMANDS true" \
  --prune-empty --tag-name-filter cat -- --all

echo "✅ Files removed from history"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "STEP 3: Clean up backup refs and garbage collect"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Removing backup refs..."
rm -rf .git/refs/original/

echo "Expiring reflog..."
git reflog expire --expire=now --all

echo "Running garbage collection (this may take a while)..."
git gc --prune=now --aggressive

echo "✅ Cleanup complete"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "STEP 4: Check repository size reduction"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

REPO_SIZE=$(du -sh .git | cut -f1)
echo "Current .git directory size: $REPO_SIZE"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "STEP 5: Update .gitignore to prevent future commits"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Check if .gitignore already has these patterns
if ! grep -q "^# Build artifacts" .gitignore 2>/dev/null; then
    echo "Adding build artifact patterns to .gitignore..."
    cat >> .gitignore << 'EOF'

# Build artifacts (large binaries that should never be committed)
workflowexecutor
integration-webhook-server
kubernaut
webhook.test
dynamic-toolset-server
main
remediationprocessor
/cmd/*/main
/cmd/*/*
!/cmd/*/*.go
!/cmd/*/main.go
EOF
    echo "✅ .gitignore updated"
else
    echo "✅ .gitignore already contains build artifact patterns"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ SUCCESS - Large binaries removed from git history"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📋 NEXT STEPS:"
echo ""
echo "1. Verify the files are gone from history:"
echo "   git log --all --full-history --stat | grep -E '(workflowexecutor|kubernaut|main)'"
echo "   (should show nothing or only deletions)"
echo ""
echo "2. Check current git status:"
echo "   git status"
echo ""
echo "3. Check repository size:"
echo "   du -sh .git"
echo "   (should be significantly smaller)"
echo ""
echo "4. Force push to remote (⚠️  WARNING: This rewrites remote history!):"
echo "   git push origin --force --all"
echo "   git push origin --force --tags"
echo ""
echo "5. If something goes wrong, restore from backup:"
echo "   rm -rf /Users/jgil/go/src/github.com/jordigilh/kubernaut"
echo "   cp -r $BACKUP_DIR /Users/jgil/go/src/github.com/jordigilh/kubernaut"
echo ""
echo "📁 Backup location: $BACKUP_DIR"
echo ""
echo "⚠️  IMPORTANT: After force pushing, any collaborators will need to:"
echo "   git fetch origin"
echo "   git reset --hard origin/<branch-name>"
echo ""
echo "💡 TIP: To prevent future commits of build artifacts, add to Makefile:"
echo "   .PHONY: clean"
echo "   clean:"
echo "       rm -f workflowexecutor integration-webhook-server kubernaut"
echo "       rm -f webhook.test dynamic-toolset-server main remediationprocessor"
echo ""

