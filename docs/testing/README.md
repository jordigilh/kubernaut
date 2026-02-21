# Kubernaut Testing Documentation

This directory contains comprehensive testing strategy and implementation guidance for kubernaut.

## 📋 **Quick Start**

### **🎯 PRIMARY GUIDE**
**[PYRAMID_TEST_MIGRATION_GUIDE.md](PYRAMID_TEST_MIGRATION_GUIDE.md)** - Complete 8-week migration to pyramid testing strategy (70% Unit / 20% Integration / 10% E2E)

## 📚 **Documentation Overview**

| Document | Purpose | When to Use |
|----------|---------|-------------|
| **[PYRAMID_TEST_MIGRATION_GUIDE.md](PYRAMID_TEST_MIGRATION_GUIDE.md)** | ⭐ **START HERE** - Complete pyramid strategy migration plan | Planning and implementing pyramid testing approach |
| **[INTEGRATION_E2E_NO_MOCKS_POLICY.md](INTEGRATION_E2E_NO_MOCKS_POLICY.md)** | 🚨 **MANDATORY** - Zero mocks policy for integration/E2E tests | **REQUIRED READING** - Before writing any integration or E2E tests |
| **[TESTING_GUIDELINES_TRANSFORMATION_GUIDE.md](TESTING_GUIDELINES_TRANSFORMATION_GUIDE.md)** | Testing transformation best practices | Understanding testing patterns and anti-patterns |
| **[TESTING_PATTERNS_QUICK_REFERENCE.md](TESTING_PATTERNS_QUICK_REFERENCE.md)** | Quick reference for testing patterns | Daily development reference |
| **[TESTING_MAINTENANCE_CHECKLIST.md](TESTING_MAINTENANCE_CHECKLIST.md)** | Testing maintenance guidelines | Maintaining test quality and performance |
| **[BEFORE_AFTER_EXAMPLES.md](BEFORE_AFTER_EXAMPLES.md)** | Real transformation examples | Understanding testing improvements |
| **[TEST_PLAN_TEMPLATE.md](TEST_PLAN_TEMPLATE.md)** | Reusable test plan template | **USE THIS** - When creating a test plan for any new feature or issue |
| **[TEST_CASE_SPECIFICATION_TEMPLATE.md](TEST_CASE_SPECIFICATION_TEMPLATE.md)** | IEEE 829 individual test case format | Writing detailed test case specifications |

## 🎯 **Pyramid Testing Strategy Summary**

### **Distribution Target**
- **Unit Tests (70%+)**: Maximum coverage with real business logic, external mocks only
- **Integration Tests (20%)**: Critical component interactions that require real integration
- **E2E Tests (10%)**: Essential customer-facing workflows requiring production environments

### **Current Status** (November 9, 2025)
- **Unit Tests**: 1,060 specs across 6 services (100% pass rate)
- **Integration Tests**: 40 test files across 4 services
- **E2E Tests**: 8 test files
- **Total**: 1,100+ tests with 100% BR coverage (156 BRs documented)

### **Key Principles**
1. **🚨 ZERO MOCKS in Integration/E2E**: See [INTEGRATION_E2E_NO_MOCKS_POLICY.md](INTEGRATION_E2E_NO_MOCKS_POLICY.md)
2. **Mock ONLY External Dependencies in Unit Tests**: Databases, APIs, K8s, LLM services
3. **Use 100% Real Business Logic**: All internal pkg/ components in ALL test types
4. **Maximum Unit Coverage**: Cover ALL business requirements that can be unit tested
5. **Fast Feedback**: Unit tests <10ms, total suite <15 minutes

## 🚀 **Implementation Phases**

### **Phase 1 (Weeks 1-4): Unit Test Expansion**
- Target: 31.2% → 70% unit test coverage
- Strategy: Massive unit test expansion with real business logic
- Focus: Mock infrastructure overhaul

### **Phase 2 (Weeks 5-6): Integration Test Refactoring**
- Target: Reduce integration tests to 20% of total
- Strategy: Focus on critical component interactions only
- Focus: Eliminate redundant integration tests

### **Phase 3 (Weeks 7-8): E2E Test Minimization**
- Target: Reduce E2E tests to 10% of total
- Strategy: Essential customer workflows only
- Focus: Production-like critical scenarios

## 📖 **Related Documentation**

- **Testing Framework Core**: [../TESTING_FRAMEWORK.md](../TESTING_FRAMEWORK.md)
- **Testing Strategy Rules**: [../../.cursor/rules/03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)
- **Coverage Progress**: [../../UNIT_TEST_COVERAGE_PROGRESS.md](../../UNIT_TEST_COVERAGE_PROGRESS.md)
- **Confidence Assessment**: [../../UNIT_TEST_EXTENSION_CONFIDENCE_ASSESSMENT.md](../../UNIT_TEST_EXTENSION_CONFIDENCE_ASSESSMENT.md)

## 🔗 **Quick Navigation**

- **[📋 Main Documentation Index](../DOCUMENTATION_INDEX.md)**
- **[🏗️ Architecture Guides](../architecture/)**
- **[📋 Business Requirements](../requirements/)**
- **[🚀 Development Guides](../development/)**

---

*For questions about testing strategy or implementation, refer to the pyramid migration guide or project guidelines.*
