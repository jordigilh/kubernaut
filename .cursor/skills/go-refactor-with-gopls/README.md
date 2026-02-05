# Go Refactoring with gopls - Cursor Skill

Comprehensive Cursor skill for type-safe Go refactoring using gopls.

## Quick Start

### Important: No Server Setup Required

This skill uses **gopls CLI mode**, which is completely independent from VS Code/Cursor's gopls server. Each command:
- Starts a fresh gopls instance
- Executes the operation
- Exits

**You don't need to start a gopls server.** Just ensure gopls is installed:
```bash
gopls version  # Should show v0.21.0 or later
# If not installed: go install golang.org/x/tools/gopls@latest
```

### Using the Skill (AI-Triggered)

Simply ask the AI to perform refactoring operations:

```
"Rename the Add function to Sum"
"Move oldpkg to pkg/math/calculations"
"Extract this code into a new function"
"Inline this function call"
```

The AI will automatically read this skill and use gopls for type-safe refactoring.

### Using the Helper Script (Manual)

```bash
# Make script executable (if not already)
chmod +x gopls-helper.sh

# Rename a function
./gopls-helper.sh rename pkg/calc/math.go 10 6 Sum

# Move a package
./gopls-helper.sh move-package pkg/oldpkg/file.go newpkg

# List available actions
./gopls-helper.sh list-actions pkg/calc/math.go 10 6

# Show help
./gopls-helper.sh --help
```

---

## Files in This Skill

| File | Purpose |
|------|---------|
| `SKILL.md` | Main skill instructions for AI (always read first) |
| `EXAMPLES.md` | Real-world refactoring examples |
| `GOPLS_REFERENCE.md` | Complete gopls capabilities reference |
| `gopls-helper.sh` | Bash helper script for common operations |
| `gopls-helper.bats` | BATS test suite for helper script |
| `README.md` | This file - overview and testing guide |

---

## Testing

### Prerequisites

Install BATS (Bash Automated Testing System):

```bash
# macOS
brew install bats-core

# Ubuntu/Debian
sudo apt-get install bats

# Or install from source
git clone https://github.com/bats-core/bats-core.git
cd bats-core
sudo ./install.sh /usr/local
```

### Running Tests

```bash
# Run all tests
bats gopls-helper.bats

# Run specific test
bats gopls-helper.bats -f "rename command succeeds"

# Verbose output
bats gopls-helper.bats --verbose

# TAP format (for CI)
bats gopls-helper.bats --tap
```

### Expected Output

```
✓ shows help when called with --help
✓ shows help when called with -h
✓ shows help when called with no arguments
✓ shows help when called with unknown command
✓ fails when gopls is not installed
✓ warns when not in git repository
✓ rename command requires 4 arguments
✓ rename command fails when file does not exist
✓ rename command succeeds with valid arguments
✓ rename command fails when gopls fails
✓ rename command skips validation with --no-validate
✓ rename command detects build failure after refactoring
...

45 tests, 0 failures
```

### Test Coverage

The BATS test suite covers:

- ✅ **Command Parsing**: All commands and argument validation
- ✅ **Error Handling**: Missing files, invalid args, gopls failures
- ✅ **Success Cases**: All operations with mocked gopls/go
- ✅ **Validation**: Build and test checks
- ✅ **Edge Cases**: Spaces in paths, empty files, git integration
- ✅ **Help Text**: Accuracy and completeness
- ✅ **Safety Features**: Git warnings, prerequisite checks

### CI Integration

Add to `.github/workflows/ci.yml`:

```yaml
- name: Test gopls helper script
  run: |
    cd .cursor/skills/go-refactor-with-gopls
    bats gopls-helper.bats --tap
```

---

## Usage with kubernaut

### Integration with TDD Workflow

**REFACTOR Phase** (after GREEN):

```bash
# You have working but poorly named code
func calc(x, y int) int { return x + y }

# Use gopls to refactor
./gopls-helper.sh rename pkg/math/operations.go 10 6 Add

# Validate tests still pass
./gopls-helper.sh validate
```

### Pre-commit Hook

Add to `.githooks/pre-commit`:

```bash
#!/bin/bash
# Validate build after refactoring
.cursor/skills/go-refactor-with-gopls/gopls-helper.sh validate
```

---

## Troubleshooting

### Test Failures

**Issue**: Tests fail with "gopls not found"

**Solution**: Tests use mocked gopls, but some may need real toolchain:
```bash
# Skip integration tests
bats gopls-helper.bats -f "^(?!integration:)"
```

**Issue**: Tests fail with "git not initialized"

**Solution**: BATS creates temporary git repos automatically. Ensure git is installed:
```bash
git --version
```

### Script Issues

**Issue**: "no identifier found"

**Solution**: Double-check line:column position:
```bash
# Use cat -n to see line numbers
cat -n pkg/calc/math.go | grep "func Add"

# Column is 0-indexed from start of line
# "func Add(" - 'A' is at column 5
```

**Issue**: Package move creates empty directory

**Solution**: Revert and retry with correct position:
```bash
git checkout -- .
./gopls-helper.sh move-package pkg/oldpkg/file.go:1:8 newpkg
```

---

## Development

### Adding New Commands

1. Add command case to `gopls-helper.sh`:
```bash
case "$COMMAND" in
    new-command)
        # Implementation
        ;;
esac
```

2. Add tests to `gopls-helper.bats`:
```bash
@test "new-command works correctly" {
    create_mock_gopls 0
    run bash "$HELPER_SCRIPT" new-command args...
    [ "$status" -eq 0 ]
}
```

3. Update help text and documentation

4. Run tests: `bats gopls-helper.bats`

### Test Best Practices

- Use `create_mock_gopls` and `create_mock_go` for isolation
- Test both success and failure paths
- Verify error messages are helpful
- Check exit codes
- Test edge cases (empty files, spaces in paths)

---

## FAQ

### Q: Do I need to start a gopls server?

**A: No!** This skill uses gopls **CLI mode**, not server mode. Each command is self-contained:

```bash
gopls rename -w file.go:10:5 NewName
# ↑ Starts gopls, executes, exits (< 100ms)
```

VS Code/Cursor runs its own gopls **server** for IDE features (autocomplete, hover, etc.). That's completely separate and doesn't interfere with CLI operations.

### Q: Will this conflict with VS Code/Cursor's gopls?

**A: No.** The CLI commands and VS Code's gopls server are independent:
- **CLI**: Used by this skill for refactoring (one-shot operations)
- **Server**: Used by VS Code for IDE features (long-running)

They can run simultaneously without issues.

### Q: Why not reuse VS Code's gopls server?

**A: CLI is simpler and just as fast.**

Using the server would require:
- Finding the server port/socket
- JSON-RPC communication
- Connection management
- Parsing responses

CLI is much simpler: `gopls rename -w file.go:10:5 Name`

Plus, gopls CLI startup is < 100ms, so there's no performance benefit to reusing the server.

### Q: How do I know gopls is installed?

```bash
gopls version
# Output: golang.org/x/tools/gopls v0.21.0

# If not found:
go install golang.org/x/tools/gopls@latest
```

### Q: Can I run refactoring while VS Code is open?

**A: Yes!** The skill's CLI operations are completely independent. You can:
- Have VS Code/Cursor open with gopls running
- Use this skill (AI or manual script)
- Everything works together

The only consideration: commit your changes first so you can review what gopls modified.

---

## References

- [gopls Documentation](https://go.dev/gopls/features/transformation)
- [BATS Documentation](https://bats-core.readthedocs.io/)
- [Kubernaut TDD Guidelines](../../../docs/development/methodology/APDC_FRAMEWORK.md)
- [LSP Specification](https://microsoft.github.io/language-server-protocol/)

---

## Contributing

When updating this skill:

1. Keep `SKILL.md` under 500 lines (use progressive disclosure)
2. Add examples to `EXAMPLES.md`
3. Update tests in `gopls-helper.bats`
4. Verify all tests pass: `bats gopls-helper.bats`
5. Update this README if adding new features

---

## License

Part of the kubernaut project. See repository LICENSE.
