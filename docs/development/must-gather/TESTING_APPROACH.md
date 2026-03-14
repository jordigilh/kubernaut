# Must-Gather Testing Approach

**Last Updated**: 2026-01-04
**Status**: ✅ **Container-Based Testing** (Host-Agnostic)

---

## 🎯 **Testing Philosophy**

**Principle**: Tests should be **host-agnostic** and run in the same environment as production.

**Why Container-Based Testing?**
1. ✅ **Consistency**: Same results on macOS, Linux, CI, and developer machines
2. ✅ **Production Parity**: Tests run in UBI10 (same as production container)
3. ✅ **Isolation**: No dependency on host OS tools or versions
4. ✅ **CI-Ready**: Works identically in GitHub Actions, Jenkins, GitLab CI
5. ✅ **Reproducibility**: Eliminates "works on my machine" issues

---

## 🐳 **Container-Based Testing (Default)**

### Primary Command: `make test`

**What it does**:
1. Builds the must-gather container image (`localhost/must-gather:test`)
2. Mounts test scripts and source code into container
3. Installs `bats` inside the UBI10 container
4. Runs all unit tests inside the container
5. Reports results to host

**Usage**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut/cmd/must-gather

# Run all tests (recommended)
make test

# Verbose output (shows each test execution)
make test-container-verbose

# CI pipeline (lint + container tests)
make ci
```

**What's tested**:
- ✅ Scripts execute correctly in UBI10 environment
- ✅ All tools available (kubectl, jq, tar, gzip, bash)
- ✅ Linux-specific behavior (GNU date, etc.)
- ✅ Production-like environment

**Container Environment**:
```
Base:       registry.access.redhat.com/ubi10/ubi:latest
OS:         Red Hat Enterprise Linux 9
Shell:      bash 5.1+
Tools:      kubectl 1.31, jq 1.7, GNU coreutils, tar, gzip
Test Tool:  bats (installed at test time)
```

---

## ⚡ **Local Testing (Fast Iteration)**

### Alternative Command: `make test-unit-local`

**Use case**: Quick iteration during development (NOT for CI or final validation)

**Caveat**: ⚠️ Results may differ from container due to:
- macOS vs Linux differences (date commands, file paths)
- Different tool versions (Homebrew vs UBI packages)
- Host-specific environment variables

**Usage**:
```bash
# Fast local iteration (use sparingly)
make test-unit-local
```

**When to use**:
- 🟢 Quick sanity check during script writing
- 🟢 Debugging test logic (faster cycle time)
- 🔴 Final validation before commit (use `make test` instead)
- 🔴 CI pipeline (must use `make test`)

---

## 📊 **Comparison: Container vs Local Testing**

| Aspect | `make test` (Container) | `make test-unit-local` |
|--------|-------------------------|------------------------|
| **Environment** | UBI10 (production) | macOS/Linux (host) |
| **Consistency** | ✅ Identical across machines | ❌ Host-dependent |
| **CI-Ready** | ✅ Yes | ❌ No |
| **Speed** | 🐢 ~30s (build + test) | ⚡ ~5s (test only) |
| **Tools** | UBI packages (GNU) | Homebrew (BSD on Mac) |
| **Date cmd** | GNU `date` | BSD `date` (Mac) |
| **Recommended for** | Commits, CI, validation | Quick iteration |

---

## 🔄 **Typical Development Workflow**

### Recommended Workflow

```bash
# 1. Edit scripts or tests
vim collectors/datastorage.sh
vim test/test_datastorage.bats

# 2. Quick local check (optional, for speed)
make test-unit-local   # Fast sanity check

# 3. Full validation in container (REQUIRED before commit)
make test              # Container-based, host-agnostic

# 4. Lint check
make lint              # Shellcheck validation

# 5. Full validation
make validate          # Lint + container tests

# 6. Commit when all pass
git add .
git commit -m "feat: add DataStorage API collector"
```

### CI Pipeline Workflow

```bash
# CI always uses container-based testing
make ci                # Lint + container tests (no local tests)
```

---

## 🛠️ **Test Infrastructure Details**

### Container Test Execution

**Makefile target**:
```makefile
test-container: build ## Run unit tests inside container
	podman run --rm \
		--platform linux/amd64 \
		-v $(PWD)/test:/must-gather/test:ro \
		-v $(PWD)/collectors:/usr/share/must-gather/collectors:ro \
		-v $(PWD)/sanitizers:/usr/share/must-gather/sanitizers:ro \
		-v $(PWD)/utils:/usr/share/must-gather/utils:ro \
		-v $(PWD)/gather.sh:/usr/bin/gather:ro \
		-w /must-gather \
		--entrypoint bash \
		localhost/must-gather:test \
		-c "microdnf install -y bats && bats /must-gather/test/test_*.bats"
```

**Why these volume mounts?**
- ✅ **Read-only mounts** (`-v path:ro`): Tests can't modify source
- ✅ **Latest code**: No need to rebuild for each test iteration
- ✅ **Fast cycle**: Change script → run test → no rebuild needed

**Why install bats at runtime?**
- UBI10 doesn't include bats by default
- Small package (~2MB), fast to install
- Alternative: Create custom test base image (future optimization)

---

## 🚀 **CI/CD Integration**

### GitHub Actions Example

```yaml
name: Must-Gather Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install podman
        run: sudo apt-get update && sudo apt-get install -y podman

      - name: Run container-based tests
        run: |
          cd cmd/must-gather
          make test  # Always uses container
```

### GitLab CI Example

```yaml
must-gather-tests:
  image: quay.io/podman/stable
  script:
    - cd cmd/must-gather
    - make test  # Container-based, host-agnostic
```

---

## 🐛 **Troubleshooting**

### Issue: Container build fails

**Symptoms**: `make test` fails at build step

**Solution**:
```bash
# Build manually to see full error
make build

# Or build with verbose output
podman build --platform linux/amd64 -t localhost/must-gather:test .
```

### Issue: Tests pass locally but fail in container

**Cause**: Host-specific behavior (macOS vs Linux)

**Solution**: This is expected! Container tests are authoritative.
```bash
# Investigate difference
make test-unit-local    # See local behavior
make test-container-verbose  # See container behavior

# Fix script to work in Linux/UBI10 environment
vim collectors/datastorage.sh
```

**Common macOS vs Linux differences**:
- `date` command syntax (BSD vs GNU)
- File paths (`/tmp` vs `/var/tmp`)
- Tool versions (Homebrew vs UBI packages)

### Issue: Slow test execution

**Cause**: Container build + bats installation on each run

**Optimization 1**: Use local tests for iteration
```bash
# Fast iteration during development
make test-unit-local

# Final validation before commit
make test
```

**Optimization 2**: Keep container running (advanced)
```bash
# Start interactive container
podman run -it --rm \
    -v $(pwd)/test:/must-gather/test \
    localhost/must-gather:test bash

# Inside container, install bats once
microdnf install -y bats

# Run tests multiple times
bats /must-gather/test/test_*.bats
```

---

## 📋 **Testing Checklist**

### Before Every Commit

- [ ] Run `make test` (container-based) - ✅ REQUIRED
- [ ] Run `make lint` (shellcheck) - ✅ REQUIRED
- [ ] All tests passing (100%) - ✅ REQUIRED
- [ ] No linter warnings - ✅ REQUIRED

### Optional (Fast Iteration)

- [ ] Run `make test-unit-local` - 🟡 Optional for quick checks
- [ ] Interactive debugging in container - 🟡 Optional

### Before Release

- [ ] Run `make validate` (lint + container tests) - ✅ REQUIRED
- [ ] Run `make test-e2e` on Kind cluster - ✅ REQUIRED
- [ ] Security scan: `podman scan localhost/must-gather:test` - ✅ REQUIRED

---

## 🎓 **Best Practices**

### DO ✅

- ✅ **Always use `make test` for commits**: Ensures consistency
- ✅ **Trust container results over local**: Container = production
- ✅ **Write platform-agnostic scripts**: Use GNU coreutils patterns
- ✅ **Test in CI before merging**: Automated container testing

### DON'T ❌

- ❌ **Don't rely on `make test-unit-local`**: Only for quick iteration
- ❌ **Don't commit if container tests fail**: Even if local tests pass
- ❌ **Don't use macOS-specific commands**: `gdate`, BSD flags, etc.
- ❌ **Don't skip `make test` to save time**: Consistency > speed

---

## 📚 **References**

### Related Documentation

- **Makefile**: `cmd/must-gather/Makefile` - Test targets and usage
- **Dockerfile**: `cmd/must-gather/Dockerfile` - Container environment
- **Test Plan**: `docs/development/must-gather/TEST_PLAN_MUST_GATHER_V1_0.md`
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`

### External References

- [Bats Core Documentation](https://bats-core.readthedocs.io/)
- [Red Hat UBI Documentation](https://developers.redhat.com/products/rhel/ubi)
- [OpenShift Must-Gather Pattern](https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html)

---

## 💡 **Future Enhancements**

### Potential Optimizations

1. **Custom Test Base Image**: Pre-install bats in base image
   ```dockerfile
   FROM registry.access.redhat.com/ubi10/ubi:latest
   RUN microdnf install -y bats
   # ... rest of must-gather tools
   ```
   **Benefit**: Faster test execution (~5s saved per run)

2. **Parallel Test Execution**: Run test files in parallel
   ```bash
   bats --jobs 4 /must-gather/test/test_*.bats
   ```
   **Benefit**: ~2x faster on multi-core machines

3. **Test Result Caching**: Cache test results based on script checksums
   **Benefit**: Skip unchanged tests in CI

---

**Established**: 2026-01-04
**Review Date**: V1.1 planning (Q2 2026)
**Owner**: Kubernaut Platform Team

