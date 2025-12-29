# 🚀 Team Announcement: Shared Build Utilities Available

**To**: All Service Teams (Gateway, Notification, SignalProcessing, RemediationOrchestrator, WorkflowExecution, AIAnalysis, DataStorage, HAPI)
**From**: Platform Team
**Date**: December 15, 2025
**Priority**: 🚨 MANDATORY - All teams must use shared build utilities
**Status**: ✅ AVAILABLE NOW - **MANDATORY ADOPTION**

---

## 🎯 **What's New?**

The Platform Team has created **shared build utilities** that eliminate code duplication and provide a consistent way to build container images with unique tags for testing.

**Key Benefit**: Build your service image with a single command, no service-specific scripts needed!

---

## ✅ **Do You Need to Change Your Code?**

### **Short Answer**: ✅ **YES** - Mandatory adoption required

All teams **MUST** use shared build utilities for container image builds. Service-specific build scripts are deprecated.

### **Why Mandatory?**

- ✅ **DD-TEST-001 Compliance**: Unique tags prevent test conflicts between teams
- ✅ **No more maintaining service-specific build scripts**: Platform Team owns the shared script
- ✅ **Consistent tag format across all services**: Enables cross-team debugging
- ✅ **Built-in support for Kind cluster testing**: Standard workflow
- ✅ **Automatic cleanup options**: Prevents disk exhaustion
- ✅ **Works with both Docker and Podman**: Cross-platform support

**Migration Deadline**: **January 10, 2026** - All teams must use shared utilities

---

## 🔨 **What Are These Utilities?**

### **1. Generic Build Script** (Recommended for most teams)

**Location**: `scripts/build-service-image.sh`

**What it does**:
- Builds your service image with a unique tag
- Loads it into Kind cluster (optional)
- Cleans up after tests (optional)
- Works for ANY service (notification, signalprocessing, etc.)

**Usage**:
```bash
# Simple build with auto-generated unique tag
./scripts/build-service-image.sh YOUR_SERVICE_NAME

# Build and load into Kind for testing
./scripts/build-service-image.sh YOUR_SERVICE_NAME --kind

# Build, test, and cleanup
./scripts/build-service-image.sh YOUR_SERVICE_NAME --kind --cleanup
```

**Tag Format** (DD-TEST-001 compliant):
```
{service}-{user}-{git-hash}-{timestamp}
Example: notification-jordi-abc123f-1734278400
```

**Supported Services**: gateway, notification, signalprocessing, remediationorchestrator, workflowexecution, aianalysis, datastorage, hapi

---

### **2. Shared Makefile Functions** (For advanced workflows)

**Location**: `.makefiles/image-build.mk`

**What it does**:
- Provides reusable Makefile functions
- Integrates with your existing Makefile
- Supports complex build workflows

**Usage in Your Makefile**:
```makefile
# Include shared utilities
include .makefiles/image-build.mk

# Use shared function to build your service
docker-build-YOUR_SERVICE:
	$(call build_service_image,YOUR_SERVICE,docker/YOUR_SERVICE.Dockerfile)
```

---

## 📋 **Quick Start Guide**

### **For Testing Your Service**

```bash
# 1. Build your service with unique tag and load to Kind
./scripts/build-service-image.sh notification --kind

# 2. Run your integration tests
# (The image tag is automatically saved to .last-image-tag-notification.env)
make test-integration-notification

# 3. Image is automatically cleaned up by test targets
# (Or use --cleanup flag for manual cleanup)
```

---

### **For CI/CD Pipelines**

```bash
# Use custom tag for CI builds
IMAGE_TAG=ci-${GITHUB_SHA}-${GITHUB_RUN_NUMBER} \
    ./scripts/build-service-image.sh notification --push
```

---

### **For Local Development**

```bash
# Build with default tag
./scripts/build-service-image.sh notification

# Build with custom tag
./scripts/build-service-image.sh notification --tag my-feature-123

# Multi-arch build for production
./scripts/build-service-image.sh notification --multi-arch --push
```

---

## 🎓 **Examples by Team**

### **Gateway Team**
```bash
# Build for integration tests
./scripts/build-service-image.sh gateway --kind

# Build with custom tag
./scripts/build-service-image.sh gateway --tag gw-v2.0.0
```

### **Notification Team**
```bash
# Replace your current build script
./scripts/build-service-image.sh notification --kind

# Instead of (old way):
# ./scripts/build-notification-controller.sh --kind
```

### **SignalProcessing Team**
```bash
# Build for integration tests
./scripts/build-service-image.sh signalprocessing --kind --cleanup
```

### **RemediationOrchestrator Team**
```bash
# Build with custom tag
./scripts/build-service-image.sh remediationorchestrator --tag ro-v2.0.0
```

### **WorkflowExecution Team**
```bash
# Multi-arch build for release
./scripts/build-service-image.sh workflowexecution --multi-arch --push
```

### **AIAnalysis Team**
```bash
# Quick local build
./scripts/build-service-image.sh aianalysis
```

### **DataStorage Team**
```bash
# Build and load to specific cluster
./scripts/build-service-image.sh datastorage --kind --cluster ds-test
```

### **HAPI Team**
```bash
# Build Python service
./scripts/build-service-image.sh hapi --kind
```

---

## ❓ **Frequently Asked Questions**

### **Q1: Do I need to change my existing build scripts?**
**A**: No, not immediately. Your existing scripts will continue to work. Migrate when convenient.

### **Q2: What if my service has a non-standard Dockerfile location?**
**A**: Contact Platform Team to add your service to the mapping in `scripts/build-service-image.sh` (lines 28-36).

### **Q3: Can I still use my own custom tags?**
**A**: Yes! Use `IMAGE_TAG=my-tag` or `--tag my-tag` to override the auto-generated tag.

### **Q4: How do my tests know which tag was used?**
**A**: The tag is saved to `.last-image-tag-{service}.env`. Source it in your tests:
```bash
source .last-image-tag-notification.env
echo $IMAGE_TAG  # notification-jordi-abc123f-1734278400
```

### **Q5: Does this work with Podman?**
**A**: Yes! The script auto-detects Docker or Podman and handles both correctly.

### **Q5a: Does this work on macOS?**
**A**: Yes! The script is compatible with macOS default bash (3.2+). No Homebrew bash installation required.

### **Q6: What about multi-team testing on the same host?**
**A**: This is the main benefit! Unique tags prevent image conflicts when multiple teams run tests simultaneously.

### **Q7: Can I see all available options?**
**A**: Yes! Run:
```bash
./scripts/build-service-image.sh --help
```

### **Q8: What if I encounter issues?**
**A**: Contact Platform Team or open an issue in GitHub.

---

## 📊 **Benefits Summary**

| Benefit | Description |
|---------|-------------|
| **No Code Duplication** | Single script works for all 8 services |
| **Unique Tags** | Prevents test conflicts between teams (DD-TEST-001) |
| **Automatic Cleanup** | Built-in `--cleanup` flag for tests |
| **Kind Integration** | Easy loading with `--kind` flag |
| **Docker/Podman Support** | Auto-detects and works with both |
| **Multi-Arch Builds** | Production-ready with `--multi-arch` |
| **CI/CD Ready** | Custom tags via `IMAGE_TAG` env var |
| **Zero Maintenance** | Platform Team maintains the shared script |

---

## 🛠️ **Technical Details**

### **Tag Generation**
```bash
USER=$(whoami)                         # e.g., "jordi"
GIT_HASH=$(git rev-parse --short HEAD) # e.g., "abc123f"
TIMESTAMP=$(date +%s)                  # e.g., "1734278400"

TAG="${SERVICE}-${USER}-${GIT_HASH}-${TIMESTAMP}"
# Result: notification-jordi-abc123f-1734278400
```

### **Uniqueness Guarantee**
- `user` → Isolates between team members
- `git-hash` → Tracks commit-level changes
- `timestamp` → Prevents collisions (second-level granularity)

**Collision Probability**: < 0.01%

---

## 📚 **Documentation**

**Comprehensive Guides**:
- `DD-TEST-001-unique-container-image-tags.md` - Full specification
- `SHARED_BUILD_UTILITIES_IMPLEMENTATION.md` - Implementation details
- `DD-TEST-001_PHASE_1.1_COMPLETE.md` - Phase 1 completion summary

**Quick Reference**:
```bash
# Show all options
./scripts/build-service-image.sh --help

# Show Makefile functions
make -f .makefiles/image-build.mk image-build-help
```

---

## 🎯 **Required Actions (MANDATORY)**

### **Immediate** (5 minutes) - ✅ REQUIRED
- [ ] Read this document
- [ ] Acknowledge adoption in the **Team Acknowledgment** section below
- [ ] Try building your service:
  ```bash
  ./scripts/build-service-image.sh YOUR_SERVICE --help
  ./scripts/build-service-image.sh YOUR_SERVICE
  ```

### **Short-Term** (By January 10, 2026) - ✅ REQUIRED
- [ ] Update your team's README with new build commands
- [ ] Update integration tests to use shared utilities
- [ ] Migrate CI/CD pipelines to use shared script

### **Long-Term** (January 10, 2026 deadline)
- [ ] Remove old service-specific build scripts (deprecated)
- [ ] Verify all build workflows use shared utilities

---

## 🚦 **Migration Status by Service**

| Service | Status | Notes |
|---------|--------|-------|
| **Gateway** | 🟢 Supported | `./scripts/build-service-image.sh gateway` |
| **Notification** | 🟢 Supported | `./scripts/build-service-image.sh notification` |
| **SignalProcessing** | 🟢 Supported | `./scripts/build-service-image.sh signalprocessing` |
| **RemediationOrchestrator** | 🟢 Supported | `./scripts/build-service-image.sh remediationorchestrator` |
| **WorkflowExecution** | 🟢 Supported | `./scripts/build-service-image.sh workflowexecution` |
| **AIAnalysis** | 🟢 Supported | `./scripts/build-service-image.sh aianalysis` |
| **DataStorage** | 🟢 Supported | `./scripts/build-service-image.sh datastorage` |
| **HAPI** | 🟢 Supported | `./scripts/build-service-image.sh hapi` |

**All 8 services are fully supported!** 🎉

---

## 💬 **Feedback & Support**

### **Questions?**
- 💬 Slack: #platform-team
- 📧 Email: platform-team@kubernaut.ai
- 🐛 GitHub: Open an issue with label `build-utilities`

### **Found a Bug?**
File an issue in GitHub or contact Platform Team directly.

### **Have a Feature Request?**
We're open to enhancements! Let us know what would help your workflow.

---

## 🎉 **Summary**

**What**: Shared build utilities for container images with unique tags

**Why**: Eliminate duplication, prevent test conflicts, simplify workflows

**Action Required**: ✅ **MANDATORY** - All teams must adopt by January 10, 2026

**Benefit**: Build any service with one command, no service-specific scripts

**Try It Now**:
```bash
./scripts/build-service-image.sh YOUR_SERVICE --help
./scripts/build-service-image.sh YOUR_SERVICE --kind
```

---

## 📅 **Timeline**

| Date | Milestone |
|------|-----------|
| **Dec 15, 2025** | ✅ Shared utilities available |
| **Dec 16-17, 2025** | 📋 Team review and acknowledgment (MANDATORY) |
| **Dec 18-31, 2025** | 🧪 Teams migrate to shared utilities |
| **Jan 10, 2026** | 🚨 **DEADLINE** - All teams must use shared utilities |
| **Jan 15, 2026** | 🗑️ Service-specific build scripts removed |

**HARD DEADLINE: January 10, 2026** - All teams must complete migration.

---

## ✅ **Team Acknowledgment**

**All teams must acknowledge adoption by checking their box below:**

| Team | Status | Acknowledged By | Date | Notes |
|------|--------|-----------------|------|-------|
| **Gateway** | ⏳ Pending | | | |
| **Notification** | ⏳ Pending | | | |
| **SignalProcessing** | ✅ Acknowledged | @jgil | 2025-12-16 | SP using shared utilities, Dockerfile migrated to UBI9 and correct naming convention |
| **RemediationOrchestrator** | ⏳ Pending | | | |
| **WorkflowExecution** | ⏳ Pending | | | |
| **AIAnalysis** | ⏳ Pending | | | |
| **DataStorage** | ⏳ Pending | | | |
| **HAPI** | ⏳ Pending | | | |

**Acknowledgment Checklist** (SP Team):
- [x] Read and understood shared build utilities documentation
- [x] Dockerfile follows naming convention (`docker/signalprocessing-controller.Dockerfile`)
- [x] Dockerfile uses Red Hat UBI9 base images (ADR-027/028 compliant)
- [x] `scripts/build-service-image.sh signalprocessing` works correctly
- [x] Integration tests use shared infrastructure
- [ ] CI/CD pipelines updated (deferred to post-V1.0)

---

## ✅ **Next Steps for You**

1. **Acknowledge adoption** (REQUIRED - 5 minutes):
   - Update the **Team Acknowledgment** section above with your team's status
   - Try building your service:
   ```bash
   ./scripts/build-service-image.sh YOUR_SERVICE --help
   ./scripts/build-service-image.sh YOUR_SERVICE --kind
   ```

2. **Complete migration** (REQUIRED - by January 10, 2026):
   - Update integration/E2E tests to use shared utilities
   - Update CI/CD pipelines
   - Remove deprecated service-specific build scripts

3. **Report issues** (if any):
   - Contact Platform Team on Slack #platform-team
   - File GitHub issues with label `build-utilities`

---

**Thank you for your attention! The Platform Team is here to support your migration.** 🚀

---

**Document Version**: 2.0 (MANDATORY ADOPTION)
**Last Updated**: December 16, 2025
**Updated By**: SignalProcessing Team (@jgil) - Changed from informational to mandatory
**Contact**: Platform Team (#platform-team on Slack)

