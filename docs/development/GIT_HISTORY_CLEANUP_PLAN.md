# Git History Cleanup Plan - Remove .env Files with Credentials

**Date**: October 9, 2025
**Status**: EXECUTING
**Critical**: This will rewrite git history

---

## Files Found in Git History

```bash
$ git log --all --pretty=format: --name-only --diff-filter=A | grep -E "^\.env"
```

**Results**:
- ❌ `.env.development` - **REMOVE** (contains credentials)
- ❌ `.env.development.backup` - **REMOVE** (contains credentials)
- ✅ `.env.example` - **KEEP** (template only, no credentials)
- ❌ `.env.external-deps` - **REMOVE** (contains credentials)
- ❌ `.env.integration` - **REMOVE** (contains credentials)

---

## Execution Plan

### Step 1: Backup Current State ✅
```bash
# Create backup branch
git branch backup-before-filter-repo-$(date +%Y%m%d-%H%M%S)
```

### Step 2: Remove Credential Files from History
```bash
# Use git-filter-repo to remove files
git filter-repo --path .env.development --invert-paths
git filter-repo --path .env.development.backup --invert-paths
git filter-repo --path .env.external-deps --invert-paths
git filter-repo --path .env.integration --invert-paths
```

### Step 3: Verify Removal
```bash
# Check that files are gone from history
git log --all --oneline --name-only | grep -E "^\.env"
# Should only show .env.example
```

### Step 4: Re-add .gitignore Protection
```bash
# Ensure .gitignore is in place (already done)
# .env* files are already protected
```

---

## Important Warnings

### ⚠️ This Rewrites Git History

**Impact**:
- All commit SHAs will change
- Anyone with clones will need to re-clone
- Force push required to update remote
- Old commits become orphaned

### 🚨 Before Proceeding

**Coordinate with team if**:
- Repository is shared
- Others have active branches
- CI/CD is running

**Safe to proceed if**:
- Working alone
- Local repository only
- No one else has cloned

---

## Recovery Plan

**If something goes wrong**:
```bash
# List backup branches
git branch | grep backup-before-filter-repo

# Restore from backup
git reset --hard backup-before-filter-repo-YYYYMMDD-HHMMSS
```

---

## Execution Log

Execution will be logged below...

