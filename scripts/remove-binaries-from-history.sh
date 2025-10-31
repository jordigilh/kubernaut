#!/bin/bash

################################################################################
# Script: Remove Binaries from Git History
# Purpose: Remove large binary files from entire git history to reduce repo size
# WARNING: This rewrites git history - coordinate with team before running!
################################################################################

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}============================================================${NC}"
echo -e "${YELLOW}  Git History Binary Removal Script${NC}"
echo -e "${YELLOW}============================================================${NC}"
echo ""

# Step 1: Safety Checks
echo -e "${GREEN}[STEP 1/9] Running Safety Checks${NC}"
echo "Checking git repository status..."

if [ ! -d .git ]; then
    echo -e "${RED}ERROR: Not in a git repository root directory!${NC}"
    exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
    echo -e "${RED}ERROR: You have uncommitted changes. Please commit or stash them first.${NC}"
    git status --short
    exit 1
fi

CURRENT_BRANCH=$(git branch --show-current)
echo -e "Current branch: ${GREEN}${CURRENT_BRANCH}${NC}"
echo ""

# Step 2: Create Backup
echo -e "${GREEN}[STEP 2/9] Creating Backup${NC}"
echo "This creates a full backup of your repository..."

BACKUP_DIR="../kubernaut-backup-$(date +%Y%m%d-%H%M%S)"
echo "Backup location: ${BACKUP_DIR}"

read -p "Create backup? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    cp -r . "${BACKUP_DIR}"
    echo -e "${GREEN}✓ Backup created successfully${NC}"
else
    echo -e "${YELLOW}⚠ Skipping backup (not recommended!)${NC}"
fi
echo ""

# Step 3: Check if git-filter-repo is installed
echo -e "${GREEN}[STEP 3/9] Checking for git-filter-repo${NC}"
if command -v git-filter-repo &> /dev/null; then
    echo -e "${GREEN}✓ git-filter-repo is installed${NC}"
    USE_FILTER_REPO=true
else
    echo -e "${YELLOW}⚠ git-filter-repo not found${NC}"
    echo "git-filter-repo is the recommended tool (faster and safer)"
    echo "Install with: pip3 install git-filter-repo"
    echo ""
    read -p "Install git-filter-repo now? (y/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        pip3 install git-filter-repo
        USE_FILTER_REPO=true
        echo -e "${GREEN}✓ git-filter-repo installed${NC}"
    else
        echo "Will use git-filter-branch instead (slower, deprecated)"
        USE_FILTER_REPO=false
    fi
fi
echo ""

# Step 4: Show what will be removed
echo -e "${GREEN}[STEP 4/9] Identifying Binaries to Remove${NC}"
echo "The following patterns will be removed from history:"
echo ""

cat << 'EOF'
Root-level binaries:
  - datastorage
  - gateway
  - adapters.test
  - contextapi.test
  - datastorage.test
  - gateway.test
  - coverage.out

bin/ directory binaries:
  - bin/ai-analysis
  - bin/ai-service
  - bin/context-api
  - bin/controller-gen*
  - bin/dynamic-toolset-server
  - bin/dynamictoolset
  - bin/gateway*
  - bin/kubernaut-*
  - bin/manager
  - bin/notification-controller
  - bin/remediation-orchestrator
  - bin/remediationorchestrator
  - bin/remediationprocessor
  - bin/setup-envtest*
  - bin/test-context-performance
  - bin/webhook-service
  - bin/k8s/** (all Kubernetes test infrastructure)

Build artifacts:
  - workflowexecutor
  - integration-webhook-server
  - kubernaut
  - webhook.test
  - dynamic-toolset-server
  - main
  - remediationprocessor
  - gateway-arm64
EOF
echo ""

# Step 5: Calculate current repo size
echo -e "${GREEN}[STEP 5/9] Calculating Current Repository Size${NC}"
echo "This may take a moment..."
BEFORE_SIZE=$(du -sh .git | cut -f1)
echo -e "Current .git size: ${YELLOW}${BEFORE_SIZE}${NC}"
echo ""

# Step 6: Confirm before proceeding
echo -e "${RED}[STEP 6/9] FINAL WARNING${NC}"
echo -e "${RED}This operation will:${NC}"
echo -e "${RED}  1. Rewrite ALL git history${NC}"
echo -e "${RED}  2. Change ALL commit SHAs${NC}"
echo -e "${RED}  3. Require force-push to remote${NC}"
echo -e "${RED}  4. Affect all team members who have cloned this repo${NC}"
echo ""
echo -e "${YELLOW}After this operation, all team members must:${NC}"
echo -e "${YELLOW}  1. Delete their local clones${NC}"
echo -e "${YELLOW}  2. Re-clone from the remote${NC}"
echo ""

read -p "Are you ABSOLUTELY SURE you want to proceed? (type 'yes' to continue): " CONFIRM
if [ "$CONFIRM" != "yes" ]; then
    echo -e "${RED}Operation cancelled.${NC}"
    exit 0
fi
echo ""

# Step 7: Remove binaries from history
echo -e "${GREEN}[STEP 7/9] Removing Binaries from History${NC}"
echo "This will take several minutes depending on repository size..."
echo ""

if [ "$USE_FILTER_REPO" = true ]; then
    # Using git-filter-repo (recommended)
    echo "Using git-filter-repo method..."

    # Create paths file with all binaries to remove
    cat > /tmp/binaries-to-remove.txt << 'EOF'
datastorage
gateway
adapters.test
contextapi.test
datastorage.test
gateway.test
coverage.out
workflowexecutor
integration-webhook-server
kubernaut
webhook.test
dynamic-toolset-server
main
remediationprocessor
gateway-arm64
bin/
EOF

    git filter-repo --invert-paths --paths-from-file /tmp/binaries-to-remove.txt --force

    rm /tmp/binaries-to-remove.txt
    echo -e "${GREEN}✓ Binaries removed using git-filter-repo${NC}"

else
    # Fallback to git-filter-branch (deprecated but works)
    echo "Using git-filter-branch method (this will be slower)..."

    git filter-branch --force --index-filter \
    'git rm -rf --cached --ignore-unmatch \
        datastorage \
        gateway \
        adapters.test \
        contextapi.test \
        datastorage.test \
        gateway.test \
        coverage.out \
        workflowexecutor \
        integration-webhook-server \
        kubernaut \
        webhook.test \
        dynamic-toolset-server \
        main \
        remediationprocessor \
        gateway-arm64 \
        bin/' \
    --prune-empty --tag-name-filter cat -- --all

    echo -e "${GREEN}✓ Binaries removed using git-filter-branch${NC}"
fi
echo ""

# Step 8: Cleanup and garbage collection
echo -e "${GREEN}[STEP 8/9] Running Garbage Collection${NC}"
echo "Cleaning up and reclaiming space..."

# Remove backup refs created by filter-branch
rm -rf .git/refs/original/

# Expire all reflogs
git reflog expire --expire=now --all

# Aggressive garbage collection
git gc --prune=now --aggressive

echo -e "${GREEN}✓ Garbage collection complete${NC}"
echo ""

# Step 9: Show results
echo -e "${GREEN}[STEP 9/9] Results${NC}"
AFTER_SIZE=$(du -sh .git | cut -f1)
echo -e "Before: ${YELLOW}${BEFORE_SIZE}${NC}"
echo -e "After:  ${GREEN}${AFTER_SIZE}${NC}"
echo ""

echo -e "${GREEN}============================================================${NC}"
echo -e "${GREEN}  Operation Complete!${NC}"
echo -e "${GREEN}============================================================${NC}"
echo ""
echo -e "${YELLOW}NEXT STEPS:${NC}"
echo ""
echo "1. Verify the repository still works:"
echo "   - Check out different branches"
echo "   - Run tests"
echo "   - Build the project"
echo ""
echo "2. Force-push to remote (DANGER!):"
echo "   git push origin --force --all"
echo "   git push origin --force --tags"
echo ""
echo "3. Notify team members to re-clone:"
echo "   - Everyone must delete their local clones"
echo "   - Everyone must re-clone from remote"
echo "   - Any open PRs will need to be recreated"
echo ""
echo -e "${RED}WARNING: Do NOT run the force-push commands until you've${NC}"
echo -e "${RED}verified the repository is working correctly!${NC}"
echo ""
echo "Backup location: ${BACKUP_DIR}"
echo ""

