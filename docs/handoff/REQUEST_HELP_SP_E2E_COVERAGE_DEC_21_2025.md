# 🆘 Request: SP Team Help with DataStorage E2E Coverage

**Date**: December 21, 2025
**From**: DataStorage Team
**To**: Signal Processing Team
**Status**: 🟢 **SP Has Working Solution** | 🔴 **DS Blocked**

---

## 🎉 Congratulations!

We heard the **Signal Processing team successfully implemented E2E coverage collection**! This is a significant achievement and we'd love to learn from your experience.

---

## 🆘 Our Situation

The **DataStorage team is stuck** trying to implement the same E2E coverage feature following the [official Go blog post guide](https://go.dev/blog/integration-test-coverage).

### What Works
- ✅ 84/84 E2E tests pass
- ✅ Binary builds with `GOFLAGS=-cover`
- ✅ `GOCOVERDIR=/coverdata` set in pod
- ✅ Volume mounted and writable
- ✅ Graceful shutdown (scale to 0)

### What Fails
- ❌ **Zero coverage files written to `/coverdata`**
- ❌ `go tool covdata percent -i=./coverdata` → "no applicable files found"

---

## 📄 Detailed Problem Statement

We've prepared a comprehensive technical document for external SMEs:

**Document**: [`docs/handoff/HELP_NEEDED_E2E_COVERAGE_ISSUE_DEC_21_2025.md`](./HELP_NEEDED_E2E_COVERAGE_ISSUE_DEC_21_2025.md)

**Contents**:
- How Go coverage instrumentation works
- Side-by-side comparison: Blog post (works) vs Our setup (fails)
- Our Docker/Kubernetes environment details
- Theories about why it's failing
- Diagnostic tests we've identified

---

## 🤝 How You Can Help

### Option 1: Quick Comparison
Could you share with us:
1. **Your Dockerfile build command** (especially `-ldflags` and `GOFLAGS` usage)
2. **Your base container image** (distroless? alpine? debian?)
3. **Your Kind cluster config** (especially `extraMounts` for coverage volume)
4. **Your deployment manifest** (GOCOVERDIR env var and volume mount)
5. **Any gotchas or issues you encountered and solved**

### Option 2: Review Our Setup
Could someone from SP team review our implementation and spot what we're doing differently?

**Key Files**:
- `docker/data-storage.Dockerfile` - Our multi-stage build
- `test/infrastructure/kind-datastorage-config.yaml` - Kind config
- `test/infrastructure/datastorage.go` - Deployment with coverage volume
- `test/e2e/datastorage/datastorage_e2e_suite_test.go` - AfterSuite coverage extraction

### Option 3: Pair Programming Session
Would someone be available for a 30-minute session to help us debug live?

---

## 🔍 Key Differences We've Identified

We suspect the issue might be related to one of these differences from the simple blog post example:

| Aspect | Potential Issue |
|--------|-----------------|
| **Multi-stage Docker build** | Does `COPY --from=builder` lose coverage metadata? |
| **Static linking** | Does `-extldflags "-static"` break coverage runtime? |
| **Distroless base image** | Does coverage need libraries not in distroless? |
| **Volume mount complexity** | Host → Kind Node → Pod (3 layers vs direct) |

---

## 📊 Our Environment

```yaml
Build:
  - Dockerfile: docker/data-storage.Dockerfile
  - Builder: golang:1.22-alpine
  - Runtime: gcr.io/distroless/static-debian12:nonroot
  - Linking: CGO_ENABLED=0, -extldflags "-static"
  - GOFLAGS: -cover (when E2E_COVERAGE=true)

Runtime:
  - Kubernetes: Kind cluster
  - Container User: nonroot (uid 65532)
  - Volume: HostPath /coverdata
  - Shutdown: kubectl scale --replicas=0 (graceful SIGTERM)
```

---

## ✅ What Success Looks Like

After graceful shutdown, we should see:

```bash
$ ls ./coverdata/
covcounters.13326b42c2a107249da22f6e0d35b638.772307.1677775306041466651
covcounters.13326b42c2a107249da22f6e0d35b638.772314.1677775306053066987
...
covmeta.13326b42c2a107249da22f6e0d35b638

$ go tool covdata percent -i=./coverdata
github.com/jordigilh/kubernaut/pkg/datastorage/... coverage: XX.X% of statements
```

But currently we get:
```bash
$ ls ./coverdata/
(empty)
```

---

## 🎯 Specific Questions for SP Team

1. **Did you use multi-stage Docker builds?** If yes, how did you ensure coverage metadata survived the copy?

2. **What base image are you using?** (distroless? alpine? full debian?)

3. **Are you using static or dynamic linking?** (`-extldflags "-static"` or default?)

4. **Did you encounter any issues with coverage files not being written?** If so, how did you fix it?

5. **Any special build flags or environment variables** we should know about?

6. **Does your binary run as root or non-root user** in the container?

7. **Did you need to modify the graceful shutdown logic** to ensure coverage flush?

---

## 📞 Contact

Please respond to this document or reach out to the DataStorage team:
- We can schedule a quick sync
- Share code/configs asynchronously
- Pair debug if needed

**We'd really appreciate any insights you can share!** 🙏

---

## 📚 References

1. **[Go Blog: Code coverage for Go integration tests](https://go.dev/blog/integration-test-coverage)** - Official guide we're following
2. **[Our detailed problem statement](./HELP_NEEDED_E2E_COVERAGE_ISSUE_DEC_21_2025.md)** - Full technical details
3. **[Our troubleshooting doc](./DS_E2E_COVERAGE_TROUBLESHOOTING_DEC_21_2025.md)** - What we've tried so far

---

**Status**: 🟡 **AWAITING SP TEAM RESPONSE**

**Priority**: Medium - This is blocking E2E coverage metrics but not blocking development

**Timeline**: No immediate urgency, but we'd like to resolve this soon to complete our V1.0 maturity requirements

---

**Thank you SP team for blazing the trail on this! 🚀**

