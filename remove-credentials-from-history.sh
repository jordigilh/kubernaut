#!/bin/bash
# Script to remove vertex-ai.json from git history
# This is necessary because GitHub will reject pushes with credentials in history

set -e  # Exit on error

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Removing holmesgpt-api/.credentials/vertex-ai.json from git history"
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

# Check if the file exists in history
echo "Checking if file exists in git history..."
if git log --all --full-history --oneline -- holmesgpt-api/.credentials/vertex-ai.json | grep -q .; then
    echo "✅ File found in history (commit: $(git log --all --full-history --oneline -- holmesgpt-api/.credentials/vertex-ai.json | head -1))"
else
    echo "✅ File NOT found in history - nothing to do!"
    exit 0
fi

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
echo "STEP 2: Remove file from git history using filter-branch"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Remove the file from all commits in history
echo "Running git filter-branch..."
git filter-branch --force --index-filter \
  'git rm --cached --ignore-unmatch holmesgpt-api/.credentials/vertex-ai.json' \
  --prune-empty --tag-name-filter cat -- --all

echo "✅ File removed from history"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "STEP 3: Clean up backup refs and garbage collect"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Removing backup refs..."
rm -rf .git/refs/original/

echo "Expiring reflog..."
git reflog expire --expire=now --all

echo "Running garbage collection..."
git gc --prune=now --aggressive

echo "✅ Cleanup complete"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "STEP 4: Verify file is removed from history"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if git log --all --full-history -- holmesgpt-api/.credentials/vertex-ai.json | grep -q .; then
    echo "❌ ERROR: File still appears in history!"
    echo "Please check manually with:"
    echo "  git log --all --full-history -- holmesgpt-api/.credentials/vertex-ai.json"
    exit 1
else
    echo "✅ File successfully removed from history"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ SUCCESS - Credentials removed from git history"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📋 NEXT STEPS:"
echo ""
echo "1. Verify the file is gone from history:"
echo "   git log --all --full-history -- holmesgpt-api/.credentials/vertex-ai.json"
echo "   (should show nothing)"
echo ""
echo "2. Check current git status:"
echo "   git status"
echo ""
echo "3. Force push to remote (⚠️  WARNING: This rewrites remote history!):"
echo "   git push origin --force --all"
echo "   git push origin --force --tags"
echo ""
echo "4. If something goes wrong, restore from backup:"
echo "   rm -rf /Users/jgil/go/src/github.com/jordigilh/kubernaut"
echo "   cp -r $BACKUP_DIR /Users/jgil/go/src/github.com/jordigilh/kubernaut"
echo ""
echo "📁 Backup location: $BACKUP_DIR"
echo ""
echo "⚠️  IMPORTANT: After force pushing, any collaborators will need to:"
echo "   git fetch origin"
echo "   git reset --hard origin/main  # or their branch name"
echo ""

