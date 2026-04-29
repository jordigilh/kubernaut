# Test Plan: Session Object-Level Authorization (PR7.5)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-823-SESSION-AUTHZ-v1.1
**Feature**: Object-level authorization for session endpoints (SOC2 CC8.1)
**Version**: 1.1
**Created**: 2026-04-24
**Author**: AI Assistant + Jordi Gil
**Status**: Complete
**Branch**: `feature/pr7.5-session-authz`

---

## 1. Introduction

### 1.1 Purpose

Close the object-level authorization gap: any authenticated user can currently
access any session (status, result, cancel, snapshot, stream). This PR stores
session ownership and enforces that only the session creator can access it.
Returns 404 (not 403) for unauthorized access to prevent information leakage.

### 1.2 Objectives

1. **Ownership tracking**: Store `created_by` user identity in session metadata
2. **Object-level authz**: All 6 session endpoints check ownership before access
3. **404 for unauthorized**: Unauthorized users see "session not found" (no leakage)
4. **Dev mode parity**: When auth middleware is disabled, all access is allowed
5. **Audit attribution**: session.observed includes observer identity
6. **Denied-access audit**: access_denied event emitted for SOC2 CC8.1

---

## 2. References

- SOC2 CC8.1: Operator attribution, access control
- DD-AUTH-014: Middleware-Based SAR Authentication
- BR-SESSION-005: All session control actions are audited
- DD-AUDIT-003: Authoritative audit event registry

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Mitigation |
|----|------|--------|------------|
| R1 | Auth disabled in dev → authz always passes | Acceptable: dev-only, no auth = no ownership | Skip authz when user is empty string |
| R2 | Existing tests don't set user in context | All tests break with 404 | Guard: empty user bypasses authz (dev mode) |
| R3 | Session metadata mutation (set created_by later) | Owner can be changed | created_by set only in StartInvestigation |

---

## 4. Scope

### 4.1 Features to be Tested

- Store `created_by` from `auth.GetUserFromContext(ctx)` in metadata
- Authz guard on: status, result, cancel, snapshot, stream
- 404 response when user != owner for session endpoints
- Bypass when user is empty (auth middleware disabled)
- Audit: observer identity in session.observed event
- Audit: access_denied event on unauthorized access attempts

### 4.2 Features Not to be Tested

- Auth middleware itself (DD-AUTH-014, separate test suite)
- Admin/role-based override (future)
- Rate limiting (PR9)

---

## 5. Test Scenarios

| BR ID | Business Outcome | Tier | Test ID | Status |
|-------|------------------|------|---------|--------|
| SOC2 CC8.1 | Session created_by stored in metadata | Unit | UT-KA-823-A01 | Pass |
| SOC2 CC8.1 | Owner can access their session status | Unit | UT-KA-823-A02 | Pass |
| SOC2 CC8.1 | Non-owner gets 404 on session status | Unit | UT-KA-823-A03 | Pass |
| SOC2 CC8.1 | Non-owner gets 404 on cancel | Unit | UT-KA-823-A04 | Pass |
| SOC2 CC8.1 | Non-owner gets 404 on snapshot | Unit | UT-KA-823-A05 | Pass |
| SOC2 CC8.1 | Non-owner gets 404 on stream | Unit | UT-KA-823-A06 | Pass |
| SOC2 CC8.1 | Non-owner gets 404 on result | Unit | UT-KA-823-A07 | Pass |
| SOC2 CC8.1 | Empty user (no auth) bypasses authz | Unit | UT-KA-823-A08 | Pass |
| SOC2 CC8.1 | session.observed includes observer identity | Unit | UT-KA-823-A09 | Pass |
| SOC2 CC8.1 | Denied access emits audit event | Unit | UT-KA-823-A10 | Pass |

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan |
| 1.1 | 2026-04-24 | Added A10 (access_denied audit); updated status to Complete |
