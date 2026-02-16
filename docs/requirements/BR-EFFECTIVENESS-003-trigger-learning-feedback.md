# BR-EFFECTIVENESS-003: Trigger Learning Feedback Loops

> **ARCHIVED** (February 2026)
>
> This BR describes learning feedback loops that align with EM Level 2 (V1.1), not Level 1.
> The event publishing and AI subscription design described here may be revisited during V1.1
> planning, but the implementation approach will be reviewed against DD-017 v2.0's audit-trace
> architecture.
>
> **Authoritative source**: `docs/architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md` (v2.0)
> **V1.1 scope**: Level 2 AI-Powered Analysis (HolmesGPT PostExec)

**Business Requirement ID**: BR-EFFECTIVENESS-003
**Category**: Effectiveness Monitor Service
**Priority**: P2
**Target Version**: V2
**Status**: ⚠️ ARCHIVED — Confirmed V1.1 scope, revisit during V1.1 planning
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 requires continuous learning feedback loops to notify AI Service when playbook effectiveness changes significantly. The Effectiveness Monitor must trigger events when playbooks degrade, improve, or require attention to enable adaptive AI behavior.

**Current Limitations**:
- ❌ No automated notifications to AI Service for effectiveness changes
- ❌ AI cannot adapt to playbook degradation in real-time
- ❌ Manual coordination required for AI algorithm updates
- ❌ Missing feedback loop for continuous AI improvement

**Impact**:
- AI continues using degraded playbooks until manual intervention
- No automated learning feedback
- Delayed response to effectiveness changes
- Cannot implement ADR-033 continuous learning vision

---

## 🎯 **Business Objective**

**Trigger automated feedback loops to AI Service when playbook effectiveness changes significantly, enabling adaptive AI behavior and continuous improvement.**

### **Success Criteria**
1. ✅ Publishes events when playbook success rate drops >10%
2. ✅ Publishes events when new version outperforms old version
3. ✅ AI Service subscribes to effectiveness events
4. ✅ Configurable thresholds for event triggers
5. ✅ Event payload includes trend data and recommendations

---

## 🔧 **Functional Requirements**

### **FR-EFFECTIVENESS-003-01: Event Publishing**

**Event Types**:
- `playbook_effectiveness_degradation`: Success rate drops >10%
- `playbook_effectiveness_improvement`: Success rate increases >10%
- `playbook_version_recommendation`: New version significantly better

**Implementation**:
```go
// PublishEffectivenessEvent publishes event to AI Service
func (em *EffectivenessMonitor) PublishEffectivenessEvent(ctx context.Context, event EffectivenessEvent) error {
    payload, _ := json.Marshal(event)
    return em.eventBus.Publish(ctx, "effectiveness.events", payload)
}
```

**Acceptance Criteria**:
- ✅ Publishes events when thresholds exceeded
- ✅ Includes playbook_id, version, current_success_rate, previous_success_rate
- ✅ Includes recommended_action (deprecate, promote, investigate)

---

### **FR-EFFECTIVENESS-003-02: AI Service Subscription**

**Implementation**:
```go
// AI Service subscribes to effectiveness events
func (ai *AIService) SubscribeToEffectivenessEvents() {
    ai.eventBus.Subscribe("effectiveness.events", func(payload []byte) {
        var event EffectivenessEvent
        json.Unmarshal(payload, &event)

        // Update AI selection weights
        ai.UpdatePlaybookWeight(event.PlaybookID, event.EffectivenessScore)
    })
}
```

**Acceptance Criteria**:
- ✅ AI receives events within 10 seconds
- ✅ AI updates playbook selection weights
- ✅ AI logs effectiveness events for audit

---

## 🚀 **Implementation Phases**

### **Phase 1: Event Publisher** (Day 19 - 3 hours)
- Implement event publishing logic
- Add configurable thresholds
- Unit tests

### **Phase 2: AI Subscription** (Day 19 - 2 hours)
- Implement event subscription in AI Service
- Add weight update logic
- Integration tests

**Total Estimated Effort**: 5 hours (0.625 days)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V2**
**Date**: November 5, 2025
**Decision**: Implement as P2 priority (V2 feature)
**Rationale**: Enables adaptive AI behavior and continuous learning
**Approved By**: Architecture Team

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V2 Implementation

