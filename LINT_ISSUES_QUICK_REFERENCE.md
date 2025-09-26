# Kubernaut Lint Issues - Quick Reference

## 📊 **Current Status**
- **Total Issues**: 89 (down from 160)
- **Critical Issues**: 0 ✅
- **Build Status**: Fully Functional ✅

## 🎯 **Issue Breakdown**
| Type | Count | Priority | Description |
|------|-------|----------|-------------|
| **staticcheck** | 50 | Medium | Code style, dot imports, suggestions |
| **unused** | 37 | Low | Functions kept for future features |
| **ineffassign** | 2 | Low | False positives |

## ⚡ **Quick Fixes**

### **1. Dot Imports (25+ issues)**
```bash
# Add nolint to test files
find . -name "*_test.go" -exec grep -l "github.com/onsi" {} \;
# Add: //nolint:revive after import
```

### **2. Type Inference (5+ issues)**
```bash
# Change: var name Type = value
# To: name := value
golangci-lint run | grep "ST1023"
```

### **3. Unused Functions (37 issues)**
```bash
# Add above function: //nolint:unused
golangci-lint run | grep "unused" | grep "func"
```

## 🛠 **Commands**
```bash
# Status check
golangci-lint run | wc -l

# Build verification
go build ./...

# Issue breakdown
golangci-lint run | tail -5
```

## 📁 **Key Files**
- Test utilities: `pkg/testutil/`, `test/`
- Workflow engine: `pkg/workflow/engine/`
- AI components: `pkg/ai/`

## 🎯 **Target**
Reduce from 89 to <30 issues while maintaining full functionality.
