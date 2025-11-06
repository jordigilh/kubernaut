# BR-AI-058: Optimize Playbook Selection Algorithm

**Business Requirement ID**: BR-AI-058
**Category**: AI/LLM Service
**Priority**: P2
**Target Version**: V2
**Status**: ✅ Approved
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

V1 AI selection uses simple highest-success-rate algorithm. V2 requires more sophisticated selection incorporating effectiveness scores, trends, execution context, and multi-objective optimization.

**Current Limitations (V1)**:
- ❌ Simple algorithm: highest success rate wins
- ❌ Ignores effectiveness trends (degrading playbooks)
- ❌ Ignores execution context (incident severity, time of day)
- ❌ No multi-objective optimization (success rate + speed + cost)

**Impact**:
- Suboptimal playbook selection
- Cannot adapt to execution context
- Missing advanced AI capabilities

---

## 🎯 **Business Objective**

**Enhance AI selection algorithm with effectiveness scores, trends, execution context, and multi-objective optimization for V2.**

### **Success Criteria**
1. ✅ Uses effectiveness scores (not just success rates)
2. ✅ Incorporates trend direction (avoids degrading playbooks)
3. ✅ Considers execution context (severity, time constraints)
4. ✅ Multi-objective optimization (success + speed + cost)
5. ✅ Configurable weights for different objectives
6. ✅ 15%+ improvement in remediation success rate vs V1

---

## 🔧 **Functional Requirements**

### **FR-AI-058-01: Effectiveness Score Integration**

**Implementation**:
```go
// V2: Use effectiveness scores instead of raw success rates
func (ai *AIService) SelectPlaybookV2(ctx context.Context, incidentType string) (*PlaybookRecommendation, error) {
    // Query effectiveness scores from Effectiveness Monitor
    playbooks, err := ai.effectivenessMonitor.GetPlaybookAnalysis(ctx, incidentType)
    if err != nil {
        return nil, err
    }

    // Sort by effectiveness_score (not raw success_rate)
    sort.Slice(playbooks, func(i, j int) bool {
        return playbooks[i].EffectivenessScore > playbooks[j].EffectivenessScore
    })

    return &PlaybookRecommendation{
        PlaybookID:        playbooks[0].PlaybookID,
        Version:           playbooks[0].Version,
        EffectivenessScore: playbooks[0].EffectivenessScore,
        SelectionReason:   "Highest effectiveness score",
    }, nil
}
```

**Acceptance Criteria**:
- ✅ Uses effectiveness_score (success rate + trend + volume + confidence)
- ✅ Avoids degrading playbooks (declining trend penalty)
- ✅ Prefers improving playbooks (trend bonus)

---

### **FR-AI-058-02: Multi-Objective Optimization**

**Optimization Formula**:
```go
// Multi-objective score = w1*effectiveness + w2*speed + w3*cost
score := (weights.Effectiveness * effectivenessScore) +
         (weights.Speed * speedScore) +
         (weights.Cost * costScore)
```

**Example Weights**:
- Critical incident: effectiveness=0.8, speed=0.2, cost=0.0
- Warning incident: effectiveness=0.6, speed=0.2, cost=0.2
- Info incident: effectiveness=0.4, speed=0.2, cost=0.4

**Acceptance Criteria**:
- ✅ Configurable weights per incident severity
- ✅ Speed score based on playbook execution time
- ✅ Cost score based on resource consumption

---

## 🚀 **Implementation Phases**

### **Phase 1: Effectiveness Score Integration** (Day 20 - 4 hours)
- Replace success_rate with effectiveness_score
- Add trend direction filtering
- Unit tests

### **Phase 2: Multi-Objective Optimization** (Day 21 - 4 hours)
- Implement multi-objective scoring
- Add configurable weights
- Integration tests

**Total Estimated Effort**: 8 hours (1 day)

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V2**
**Date**: November 5, 2025
**Decision**: Implement as P2 priority (V2 feature)
**Rationale**: Enhances AI selection for better remediation outcomes
**Approved By**: Architecture Team

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V2 Implementation

