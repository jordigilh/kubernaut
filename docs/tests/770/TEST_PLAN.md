# Test Plan: KA Tool Call Budget — Exempt Prefixes, Session Reset, and Metrics RBAC

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-770-v2
**Feature**: Exempt internal planning tools from investigation budget; reset budget per session; add metrics.k8s.io RBAC
**Version**: 2.0
**Created**: 2026-04-21
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/770-ka-tool-call-ceiling`

---

## 1. Introduction

### 1.1 Purpose

When investigating KubeNodeNotReady alerts, the KA exhausts its 30-tool-call budget before
completing RCA because internal planning tool calls (`todo_write`) consume investigation budget.
Additionally, the AnomalyDetector's counters persist across Investigate() sessions (different
Remediation Requests), causing KA to become non-functional after 1-2 incidents.

This test plan covers:
1. Exempting `todo_*` prefixed tools from the anomaly detector's total and per-tool budgets
2. Resetting the AnomalyDetector at the start of each Investigate() call (#770 critical fix)
3. Adding `metrics.k8s.io` RBAC permissions to prevent wasted tool calls on forbidden API calls
4. Distinguishing budget exhaustion from turn exhaustion in ExhaustedResult messages

### 1.2 Objectives

1. `todo_write` calls do NOT increment totalCallCount or toolCallCounts
2. `todo_write` calls are still checked for suspicious arguments
3. Investigation tools (kubectl_*, prometheus_*) still count against budgets
4. AnomalyDetector.Reset() is called at the start of Investigate() — each session gets a fresh budget
5. ExhaustedResult from anomaly detector is labeled "tool budget exhausted" (not "max turns")
6. `metrics.k8s.io` RBAC rule added to Helm chart and E2E manifest

---

## 2. References

- Issue #770: KA investigation hits tool call ceiling (30) on Node scenarios
- DD-HAPI-019-003: Anomaly detection thresholds
- BR-SAFETY-001: Tool call safety limits

---

## 3. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test |
|----|----------------------------|
| UT-KA-770-001 | `todo_write` calls do NOT count against total budget |
| UT-KA-770-002 | `todo_write` calls do NOT count against per-tool budget |
| UT-KA-770-003 | `todo_write` calls ARE checked for suspicious arguments |
| UT-KA-770-004 | Investigation tools still count normally with exempt tools present |
| UT-KA-770-005 | Custom exempt prefixes from config are respected |
| UT-KA-770-006 | DefaultAnomalyConfig includes `todo_` in ExemptPrefixes |
| UT-KA-770-007 | TotalExceeded returns false even after many exempt tool calls |
| UT-KA-770-010a | Reset at session boundary gives second investigation a full budget |
| UT-KA-770-010b | Without session-boundary reset, counter leaks across investigations (bug doc) |

---

## 4. Existing Tests Requiring Updates

None — existing tests use investigation tool names (kubectl_*, prometheus_*) which are not exempt.
All existing assertions remain valid.

---

## 5. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2026-04-21 | Add session-boundary Reset() fix and tests (UT-KA-770-010a/b) |
| 1.0 | 2026-04-21 | Initial test plan |
