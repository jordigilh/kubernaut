# NT Config Migration - Decision Required

**Date**: December 22, 2025
**Status**: 🚧 **BLOCKED - AWAITING USER DECISION**
**Context**: ADR-030 config migration for Notification service

---

## 🚨 Critical Discovery

While implementing ADR-030 configuration for Notification service, I discovered **Kubernaut services use 3 different configuration loading patterns**:

| Pattern | Services | Example |
|---------|----------|---------|
| **CONFIG_PATH env var** | DataStorage | `cfgPath := os.Getenv("CONFIG_PATH")` |
| **-config flag** | Gateway, WE, SP | `flag.String("config", "default.yaml")` |
| **Individual env vars** | Notification (current) | `os.Getenv("FILE_OUTPUT_DIR")` |

**Full Analysis**: `CONFIG_LOADING_PATTERN_INCONSISTENCY_DEC_22_2025.md`

---

## Decision Required

### **Q1: Which pattern for Notification service?**

**Option A: CONFIG_PATH (matches DataStorage)** ✅ RECOMMENDED
```go
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    logger.Error(fmt.Errorf("CONFIG_PATH not set"))
    os.Exit(1)
}
cfg, err := config.LoadFromFile(cfgPath)
```

**Benefits**:
- ✅ Kubernetes-native (ConfigMap-first)
- ✅ Simplest implementation
- ✅ Matches most mature service (DataStorage)

---

**Option B: -config flag (matches Gateway/WE/SP)**
```go
configPath := flag.String("config", "/etc/notification/config.yaml")
flag.Parse()
cfg, err := config.LoadFromFile(*configPath)
```

**Benefits**:
- ✅ Matches most services (3 out of 4)
- ✅ Familiar K8s controller pattern

---

### **Q2: Should Kubernaut standardize on one pattern?**

**Option A: Yes, standardize** ✅ RECOMMENDED
- Migrate Gateway, WE, SP to chosen pattern
- Timeline: 2-4 hours per service

**Option B: No, allow variation**
- Document both patterns as acceptable
- No migration needed

---

## My Recommendation

**Q1**: Use **CONFIG_PATH** for Notification service
- Simpler (no flag parsing)
- Kubernetes-native
- Already proven by DataStorage

**Q2**: **Yes, standardize all services** on CONFIG_PATH
- Consistency across codebase
- Clear operator experience
- Migrate Gateway, WE, SP in follow-up work

**Q3**: **Proceed with Notification now**, document need for other services

---

## Impact on Current Work

### If CONFIG_PATH Chosen:
✅ **pkg/notification/config/config.go**: Already created (uses LoadFromFile)
✅ **ADR-030 document**: Already updated with CONFIG_PATH pattern
⏸️  **cmd/notification/main.go**: Waiting for decision to implement
⏸️  **ConfigMap**: Waiting for decision to create
⏸️  **Deployment**: Waiting for decision to update

### If -config Flag Chosen:
⚠️  **pkg/notification/config/config.go**: Keep as-is (LoadFromFile works)
⚠️  **ADR-030 document**: Update to show flag pattern
⚠️  **cmd/notification/main.go**: Add flag parsing
⚠️  **ConfigMap**: Same (no change)
⚠️  **Deployment**: Use args instead of CONFIG_PATH env var

---

## Questions?

**Concerns or questions about either pattern?**
**Need clarification on trade-offs?**
**Want to see example code for both patterns?**

---

**Next Action**: User decides Q1 (pattern for NT) and Q2 (standardize or not)
**ETA**: 2-3 hours to complete NT config migration once decision made

