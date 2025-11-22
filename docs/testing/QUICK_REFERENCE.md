# Defense-in-Depth CI/CD - Quick Reference

**Authority**: `docs/testing/APPROVED_CI_CD_STRATEGY.md`

---

## 🚀 **How It Works**

### **Smart Path Detection**
- ✅ Only runs tests for changed code
- ✅ 80%+ reduction in test runs
- ✅ Faster feedback (5-30 min vs 60+ min)

### **Example Scenarios**

| Your Changes | Tests That Run | Duration |
|--------------|----------------|----------|
| **Only Data Storage** | Unit + Data Storage (integration + E2E) | ~10 min |
| **Only HolmesGPT** | Unit + HolmesGPT integration | ~3 min |
| **Only Gateway** | Unit + Gateway (integration + E2E) | ~25 min |
| **Multiple services** | Unit + All affected services | ~30 min |

---

## 📋 **Developer Workflow**

### **1. Create Draft PR** (Fast Iteration)
```bash
git checkout -b feat/my-feature
git commit -m "feat: implement feature"
git push origin feat/my-feature
# Create PR as DRAFT
```

**What Runs**:
- ✅ Unit tests (~2 min)
- ✅ Integration tests for affected services (~5-10 min)
- ❌ E2E tests skipped (save time during development)

**Total**: ~10 min

---

### **2. Iterate on Code** (Fast Feedback)
```bash
git commit -m "fix: address review comments"
git push
```

**What Runs**:
- ✅ Unit tests (~2 min)
- ✅ Integration tests (~5-10 min)
- ❌ E2E tests still skipped

**Total**: ~10 min per iteration

---

### **3. Mark Ready for Review** (Full Validation)
```bash
# On GitHub: Mark PR as "Ready for review"
```

**What Runs**:
- ✅ Unit tests (~2 min)
- ✅ Integration tests (~5-10 min)
- ✅ **E2E tests now run** (~15-20 min)

**Total**: ~30 min

---

### **4. Merge** (Confidence)
```bash
# All tests passed → Merge with confidence
```

**Coverage**:
- ✅ 70%+ unit coverage
- ✅ >50% integration coverage
- ✅ 10-15% E2E coverage
- ✅ **130-165% total** (defense-in-depth)

---

## 🎯 **What Triggers Tests**

### **Data Storage**
```yaml
Triggers:
  - pkg/datastorage/**
  - cmd/datastorage/**
  - test/integration/datastorage/**
  - test/e2e/datastorage/**
  - migrations/**
  - docker/data-storage.Dockerfile
```

### **Gateway Service**
```yaml
Triggers:
  - pkg/gateway/**
  - cmd/gateway/**
  - test/integration/gateway/**
  - test/e2e/gateway/**
```

### **HolmesGPT API**
```yaml
Triggers:
  - holmesgpt-api/**
  - test/integration/holmesgpt/**
```

### **Dynamic Toolset**
```yaml
Triggers:
  - pkg/toolset/**
  - cmd/toolset/**
  - test/integration/toolset/**
  - test/e2e/toolset/**
```

### **Notification Service**
```yaml
Triggers:
  - pkg/notification/**
  - cmd/notification/**
  - test/integration/notification/**
```

---

## 🔧 **Manual Controls**

### **Force Full Test Run**
```bash
# Push to main branch (always runs all tests)
git checkout main
git merge feat/my-feature
git push origin main
```

### **Skip E2E During Development**
```bash
# Keep PR as draft
# E2E tests automatically skipped
```

### **Run E2E Manually**
```bash
# Mark PR as "Ready for review"
# E2E tests automatically run
```

---

## 📊 **Test Tiers**

### **Tier 1: Unit Tests** (< 2 min)
- **Coverage**: 70%+ of business requirements
- **When**: Every commit
- **Purpose**: Fast feedback on business logic

### **Tier 2: Integration Tests** (5-10 min)
- **Coverage**: >50% of business requirements
- **When**: Every commit (affected services only)
- **Purpose**: Validate cross-component interactions

### **Tier 3: E2E Tests** (15-20 min)
- **Coverage**: 10-15% of business requirements
- **When**: Ready for review + main branch
- **Purpose**: Critical user journey validation

---

## ⚡ **Performance Tips**

### **Fastest Feedback** (3-10 min)
1. Keep PRs focused on single service
2. Use draft PRs during development
3. Mark ready for review only when confident

### **Avoid Slow PRs** (30+ min)
1. ❌ Don't change multiple services unnecessarily
2. ❌ Don't mark ready for review too early
3. ❌ Don't push to main branch for testing

---

## 🚨 **Troubleshooting**

### **"Tests didn't run for my changes"**
**Cause**: Path detection didn't match your changes
**Fix**: Check if your files are in the trigger paths above

### **"E2E tests are flaky"**
**Cause**: Network timing, resource contention
**Fix**: Tests automatically retry once. If still failing, check logs.

### **"Tests are taking too long"**
**Cause**: Multiple services changed or E2E running
**Fix**: Use draft PRs to skip E2E during development

---

## 📞 **Need Help?**

- **Documentation**: `docs/testing/APPROVED_CI_CD_STRATEGY.md`
- **Implementation**: `docs/testing/FINAL_IMPLEMENTATION_SUMMARY.md`
- **Workflow Files**: `.github/workflows/defense-in-depth-tests.yml`

---

## 🎯 **Quick Commands**

```bash
# Run tests locally (before pushing)
make test                          # Unit tests
make test-integration-datastorage  # Data Storage integration
make test-e2e-datastorage          # Data Storage E2E

# Check what will run in CI
git diff --name-only origin/main   # See changed files
# Match against trigger paths above

# Monitor CI run
gh run watch  # GitHub CLI
# Or check Actions tab on GitHub
```

---

**Remember**: Draft PRs = Fast feedback (10 min), Ready for review = Full validation (30 min)

