# Development Documentation

This directory contains **cross-cutting development documentation** that applies to multiple services or the overall project.

---

## Contents

### 🎯 Critical Path
- **[CRITICAL_PATH_IMPLEMENTATION_PLAN.md](./CRITICAL_PATH_IMPLEMENTATION_PLAN.md)** - Overall project implementation plan across all services

### 📊 Service-Specific Implementation
Service implementation details are organized under their respective service directories:

- **Gateway Service**: `../services/stateless/gateway-service/implementation/`
- **RemediationRequest Controller**: `../services/crd-controllers/01-remediationrequest/`
- *(Other services will follow the same pattern)*

---

## Documentation Organization Principle

### ❌ What NOT to Put Here
- Service-specific implementation plans
- Service-specific status reports
- Service-specific design decisions
- Service-specific test strategies

### ✅ What GOES Here
- Cross-service implementation plans
- Project-wide development guidelines
- Overall project roadmaps
- Multi-service coordination documents

---

## Service Documentation Pattern

Each service should have its implementation documentation under:
```
docs/services/{stateless|crd-controllers}/{service-name}/implementation/
├── README.md                    - Navigation & overview
├── phase0/                      - Phase 0 implementation docs
├── testing/                     - Testing strategy & status
├── design/                      - Design decisions
└── archive/                     - Historical documents
```

### Benefits of This Pattern
1. **Easy to Find**: All Gateway docs in one place
2. **Scalable**: Won't clutter this directory as more services are added
3. **Self-Contained**: Each service has complete documentation
4. **Historical Context**: Journey-based organization preserves decisions

---

## Example: Gateway Service Documentation

The Gateway service implementation is fully documented at:
- **Location**: `../services/stateless/gateway-service/implementation/`
- **Entry Point**: `00-HANDOFF-SUMMARY.md` (start here)
- **Organization**:
  - **phase0/**: Implementation timeline (plan → triage → status → complete)
  - **testing/**: Testing strategy (assessment → ready → execution)
  - **design/**: Technical decisions (CRD schema gaps, etc.)
  - **archive/**: Historical documents

This pattern should be followed for all future services.

---

## Current Cross-Cutting Documents

| Document | Purpose | Status |
|----------|---------|--------|
| CRITICAL_PATH_IMPLEMENTATION_PLAN.md | Overall project roadmap | ✅ Active |
| *(Add more as needed)* | | |

---

## Migration Complete ✅

All Gateway-specific documentation has been moved from this directory to:
```
docs/services/stateless/gateway-service/implementation/
```

This keeps the development directory clean and focused on cross-cutting concerns.




