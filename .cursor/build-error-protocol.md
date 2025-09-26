# Build Error Investigation Protocol - Quick Reference

## 🚀 **INSTANT COMMAND**

**Copy-paste this when you need thorough build error analysis:**

```
Before fixing any 'undefined' errors, use codebase_search and grep to analyze ALL references to the undefined symbol. Show me the complete dependency chain and ask for my approval before implementing any missing types or functions. Follow cursor rules for comprehensive analysis.
```

## 📋 **WHAT THIS TRIGGERS**

When you use this prompt, I will:

1. ✅ **Search all references** to the undefined symbol
2. ✅ **Map dependency chains** and related infrastructure
3. ✅ **Test compilation impact** of potential fixes
4. ✅ **Present options** with complete analysis
5. ✅ **Ask for your approval** before implementing anything

## 🎯 **USAGE SCENARIOS**

### Scenario A: New Build Errors
```
You: "Fix these build errors: [paste errors]"
Add: [paste quick command above]
```

### Scenario B: Undefined Symbol Errors
```
You: "undefined: SomeType"
Add: [paste quick command above]
```

### Scenario C: Integration Test Issues
```
You: "Integration tests failing to compile"
Add: [paste quick command above]
```

## ⚡ **KEYBOARD SHORTCUT OPTIONS**

### Option 1: Text Expander (macOS/Windows)
- **Trigger**: `;builderr`
- **Expansion**: [the command above]

### Option 2: VS Code Snippet
- **Trigger**: `investigate-build-error` + Tab
- **Result**: Inserts the protocol command

### Option 3: Alfred/Raycast Snippet
- **Keyword**: `build-protocol`
- **Action**: Copy command to clipboard

## 🔧 **CURSOR COMPOSER SETUP**

If you want to add this as a Cursor Composer prompt:

1. Open Cursor Settings (`Cmd/Ctrl + ,`)
2. Go to "Composer" or "Custom Instructions"
3. Add new prompt with trigger: `/investigate-build`
4. Content: [the command from above]

## 📖 **FULL RULE REFERENCE**

The complete protocol is integrated in:
`.cursor/rules/00-ai-assistant-behavioral-constraints.mdc` - **CHECKPOINT D: Build Error Investigation**

This ensures automatic enforcement of the investigation protocol for all build error scenarios as part of the core AI behavioral constraints.
