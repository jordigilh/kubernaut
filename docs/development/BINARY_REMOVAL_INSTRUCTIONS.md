# Binary Removal from Git History - Instructions

## Overview
This document provides step-by-step instructions to safely remove binary files from git history to reduce repository size.

## ⚠️ CRITICAL WARNINGS

1. **This rewrites git history** - All commit SHAs will change
2. **Team coordination required** - All team members must re-clone after this operation
3. **Backup is mandatory** - The script creates a backup, but verify it exists
4. **One-way operation** - Cannot be easily undone without the backup
5. **Requires force-push** - This affects everyone with access to the repository

## Prerequisites

### 1. Install git-filter-repo (Recommended)
```bash
# Using pip
pip3 install git-filter-repo

# Or using Homebrew (macOS)
brew install git-filter-repo

# Or using package manager (Ubuntu/Debian)
apt-get install git-filter-repo
```

**Why git-filter-repo?**
- 10-100x faster than git-filter-branch
- Safer and more reliable
- Officially recommended by Git project
- Better handling of edge cases

### 2. Verify Repository State
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Ensure working directory is clean
git status

# Commit or stash any pending changes
git add -A
git commit -m "Save work before binary removal"

# Or stash
git stash
```

## Execution Steps

### Step 1: Run the Script
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./scripts/remove-binaries-from-history.sh
```

The script will:
1. ✅ Check repository status
2. ✅ Create automatic backup
3. ✅ Install/check for git-filter-repo
4. ✅ Show what will be removed
5. ✅ Display current repo size
6. ⚠️ Ask for final confirmation
7. 🔧 Remove binaries from history
8. 🔧 Run garbage collection
9. 📊 Show before/after size

### Step 2: Verify Repository Integrity

**CRITICAL**: Do this BEFORE force-pushing!

```bash
# Check out different branches
git checkout main
git checkout feature/context-api

# Verify build still works
make build

# Run tests
make test

# Check git log
git log --oneline -20

# Verify no binaries in history
git log --all --full-history --raw -- bin/
git log --all --full-history --raw -- datastorage
git log --all --full-history --raw -- gateway
```

### Step 3: Force-Push to Remote

**⚠️ DANGER ZONE - ONLY AFTER VERIFICATION**

```bash
# Push all branches
git push origin --force --all

# Push all tags
git push origin --force --tags
```

### Step 4: Notify Team

Send this message to your team:

```
URGENT: Git History Rewrite Complete

The kubernaut repository history has been rewritten to remove large binary files.

ACTION REQUIRED FOR ALL TEAM MEMBERS:

1. Save any uncommitted work
2. Delete your local repository clone
3. Re-clone from remote:
   git clone <repository-url>
   cd kubernaut

4. Restore your work (if needed)

DO NOT:
- Try to merge with old history
- Push from old clones
- Pull into old clones

OPEN PRs:
- All open pull requests will need to be recreated
- Save your changes first, then create new PRs

Contact @<your-name> if you have questions.
```

## What Gets Removed

### Root-Level Binaries
- `datastorage`
- `gateway`
- `adapters.test`
- `contextapi.test`
- `datastorage.test`
- `gateway.test`
- `coverage.out`

### bin/ Directory (Entire directory)
All binaries including:
- Service binaries (ai-analysis, context-api, gateway, etc.)
- Kubernetes test infrastructure (etcd, kube-apiserver, kubectl)
- Development tools (controller-gen, setup-envtest)

### Build Artifacts
- `workflowexecutor`
- `integration-webhook-server`
- `kubernaut`
- `webhook.test`
- `dynamic-toolset-server`
- `main`
- `remediationprocessor`
- `gateway-arm64`

## Expected Results

### Repository Size Reduction
Typical results depend on binary accumulation:
- **Before**: Could be several hundred MB or GBs
- **After**: Should be significantly smaller (depends on code vs binary ratio)

You can check the size difference:
```bash
# Before running script
du -sh .git

# After running script and GC
du -sh .git
```

## Troubleshooting

### Problem: "git-filter-repo: command not found"
**Solution**: Install git-filter-repo (see Prerequisites), or the script will fall back to git-filter-branch

### Problem: "Cannot force-push to protected branch"
**Solution**:
1. Temporarily disable branch protection in GitHub/GitLab
2. Force-push
3. Re-enable branch protection

### Problem: Team member gets merge conflicts
**Solution**: They must delete and re-clone. Merging is not possible after history rewrite.

### Problem: CI/CD pipelines failing
**Solution**:
1. Clear any cached repository clones in CI
2. Re-run pipelines
3. Update any hardcoded commit SHAs in CI configuration

### Problem: Want to undo the operation
**Solution**:
```bash
# Restore from backup
cd ..
rm -rf kubernaut
mv kubernaut-backup-<timestamp> kubernaut
cd kubernaut
```

## Manual Alternative

If you prefer manual control, here's the git-filter-repo command:

```bash
# Create paths file
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

# Run filter-repo
git filter-repo --invert-paths --paths-from-file /tmp/binaries-to-remove.txt --force

# Cleanup
rm /tmp/binaries-to-remove.txt

# Garbage collection
rm -rf .git/refs/original/
git reflog expire --expire=now --all
git gc --prune=now --aggressive
```

## Post-Operation Checklist

- [ ] Backup exists and is accessible
- [ ] Repository builds successfully
- [ ] Tests pass
- [ ] All branches checked out successfully
- [ ] No binaries found in history
- [ ] Force-push completed
- [ ] Team notified
- [ ] All team members have re-cloned
- [ ] CI/CD pipelines working
- [ ] Open PRs recreated if needed

## Additional Resources

- [git-filter-repo documentation](https://github.com/newren/git-filter-repo)
- [Git documentation on rewriting history](https://git-scm.com/book/en/v2/Git-Tools-Rewriting-History)
- [GitHub: Removing sensitive data](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/removing-sensitive-data-from-a-repository)

## Support

If you encounter issues:
1. Check the backup exists
2. Review the troubleshooting section
3. Restore from backup if necessary
4. Contact the team before force-pushing

