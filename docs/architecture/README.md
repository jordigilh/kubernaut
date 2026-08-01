# Kubernaut Architecture Documentation

**Document Version**: 4.1
**Date**: December 1, 2025
**Status**: V1.0 Implementation Focus (8 Services - 2 deferred per DD-016, DD-017)

---

## 🎯 **Architecture Navigation Guide**

This directory contains the complete architectural blueprint for Kubernaut, an intelligent Kubernetes remediation platform. The documentation is organized into logical groups that tell the story of how Kubernaut transforms alerts into intelligent actions.

---

## 📚 **FOUNDATIONAL ARCHITECTURE**

### **The Core Story: From Vision to Implementation**

Start your journey with these foundational documents that establish Kubernaut's architectural vision and translate it into concrete implementation plans:

#### **🏗️ System Foundation**
1. **[KUBERNAUT_ARCHITECTURE_OVERVIEW.md](KUBERNAUT_ARCHITECTURE_OVERVIEW.md)** ✅ **AUTHORITATIVE**
   - **Purpose**: High-level system design and architectural principles
   - **Audience**: Architects, technical leads, stakeholders
   - **Key Content**: V1 system architecture with 10 core services, investigation vs execution separation
   - **Read Time**: 10 minutes

2. **[APPROVED_MICROSERVICES_ARCHITECTURE.md](APPROVED_MICROSERVICES_ARCHITECTURE.md)** ✅ **AUTHORITATIVE**
   - **Purpose**: Detailed microservices decomposition and service boundaries
   - **Audience**: Developers, DevOps engineers, system architects
   - **Key Content**: 10 V1 services with single responsibility principle, V2 roadmap (5 additional services)
   - **Read Time**: 20 minutes

#### **🔧 Service Specifications**
3. **[KUBERNAUT_SERVICE_CATALOG.md](KUBERNAUT_SERVICE_CATALOG.md)** ✅ **AUTHORITATIVE**
   - **Purpose**: Comprehensive service specifications and API contracts
   - **Audience**: Developers, integration engineers
   - **Key Content**: Service responsibilities, endpoints, dependencies, performance requirements
   - **Read Time**: 25 minutes

#### **🚀 Implementation Strategy**
4. **[KUBERNAUT_IMPLEMENTATION_ROADMAP.md](KUBERNAUT_IMPLEMENTATION_ROADMAP.md)** ✅ **AUTHORITATIVE**
   - **Purpose**: V1/V2 development strategy and timeline
   - **Audience**: Project managers, technical leads, stakeholders
   - **Key Content**: V1 foundation (3-4 weeks), V2 enhancements (6-8 weeks), risk assessment
   - **Read Time**: 15 minutes

---

## 🔄 **SERVICE INTEGRATION & COMMUNICATION**

### **The Integration Story: How Services Work Together**

These documents detail how Kubernaut's services communicate and integrate to create a cohesive intelligent system:

#### **🌐 Communication Architecture**
5. **[MICROSERVICES_COMMUNICATION_ARCHITECTURE.md](MICROSERVICES_COMMUNICATION_ARCHITECTURE.md)**
   - **Purpose**: Inter-service communication patterns and protocols
   - **Audience**: Developers, network engineers, DevOps
   - **Key Content**: HTTP/REST communication, service discovery, port assignments
   - **Connects To**: Service Catalog (service definitions) → Communication patterns
   - **Read Time**: 12 minutes

6. **[SERVICE_CONNECTIVITY_SPECIFICATION.md](SERVICE_CONNECTIVITY_SPECIFICATION.md)**
   - **Purpose**: Detailed connectivity requirements and network topology
   - **Audience**: Network engineers, security teams, DevOps
   - **Key Content**: Network policies, security boundaries, connectivity matrix
   - **Connects To**: Communication Architecture → Security implementation
   - **Read Time**: 10 minutes

#### **🔄 Workflow Orchestration**
7. **[WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md](WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md)**
   - **Purpose**: Workflow engine design and orchestration patterns
   - **Audience**: Developers, workflow designers
   - **Key Content**: Workflow execution, state management, error handling
   - **Connects To**: Service Catalog (Remediation Execution Engine) → Detailed implementation
   - **Read Time**: 18 minutes

8. **[RESILIENT_WORKFLOW_AI_SEQUENCE_DIAGRAM.md](RESILIENT_WORKFLOW_AI_SEQUENCE_DIAGRAM.md)**
   - **Purpose**: Visual representation of AI-driven workflow sequences
   - **Audience**: Developers, system architects
   - **Key Content**: End-to-end flow diagrams, failure scenarios, recovery patterns
   - **Connects To**: Workflow Orchestration → Visual representation
   - **Read Time**: 8 minutes

---

## 🤖 **AI & INTELLIGENCE ARCHITECTURE**

### **The Intelligence Story: From Data to Decisions**

These documents describe how Kubernaut transforms raw alert data into intelligent remediation actions through AI and machine learning:

#### **🧠 AI Integration**
9. **[effectiveness-monitor-sequence-diagrams.md](effectiveness-monitor-sequence-diagrams.md)** ⭐ **NEW**
   - **Purpose**: Visual representation of Effectiveness Monitor workflows (hybrid automated + AI)
   - **Audience**: Developers, integration engineers, architects
   - **Key Content**: Automated-only flow (99.3%), AI-enhanced flow (0.7%), decision logic, real examples
   - **Watch Strategy**: RemediationRequest CRD (DD-EFFECTIVENESS-003) for future-proof abstraction
   - **Connects To**: Service Catalog (Effectiveness Monitor) → Visual workflow documentation
   - **Read Time**: 12 minutes

11. **[AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md](AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md)**
    - **Purpose**: Dynamic context gathering and AI-driven investigations
    - **Audience**: AI engineers, context developers
    - **Key Content**: Context API design, historical intelligence, vector similarity search
    - **Connects To**: Kubernaut Agent → Context enhancement
    - **Read Time**: 22 minutes

#### **🔍 Pattern Discovery & Intelligence**
11. **[INTELLIGENCE_PATTERN_DISCOVERY_ARCHITECTURE.md](INTELLIGENCE_PATTERN_DISCOVERY_ARCHITECTURE.md)**
    - **Purpose**: ML-driven pattern recognition and learning systems
    - **Audience**: ML engineers, data scientists
    - **Key Content**: Pattern discovery algorithms, effectiveness tracking, continuous learning
    - **Connects To**: AI Context Orchestration → Advanced analytics
    - **Read Time**: 20 minutes

#### **🔧 Dynamic Toolset Management**
12. **[DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md](DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md)**
    - **Purpose**: Dynamic toolset discovery and configuration for AI agents
    - **Audience**: AI engineers, toolset developers
    - **Key Content**: Toolset discovery, dynamic configuration, AI agent integration
    - **Connects To**: Kubernaut Agent → Toolset management
    - **Read Time**: 16 minutes

## 💾 **DATA & STORAGE ARCHITECTURE**

### **The Data Story: Persistence, Performance, and Intelligence**

These documents detail how Kubernaut stores, retrieves, and leverages data for intelligent decision-making:

#### **📊 Storage Systems**
13. **[STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md](STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md)**
    - **Purpose**: Comprehensive data storage and management strategy
    - **Audience**: Database engineers, data architects
    - **Key Content**: PostgreSQL design, vector databases, data lifecycle management
    - **Connects To**: Service Catalog (Data Storage Service) → Detailed design
    - **Read Time**: 18 minutes

---

## 🔒 **OPERATIONAL & RELIABILITY ARCHITECTURE**

### **The Operations Story: Monitoring, Resilience, and Production Readiness**

These documents ensure Kubernaut operates reliably and securely in production environments:

#### **📊 Monitoring & Observability**
14. **[PRODUCTION_MONITORING.md](PRODUCTION_MONITORING.md)**
    - **Purpose**: Production monitoring, metrics, and observability strategy
    - **Audience**: DevOps engineers, SRE teams, operations
    - **Key Content**: Metrics collection, alerting, dashboards, SLA monitoring
    - **Connects To**: Service Catalog → Operational requirements
    - **Read Time**: 14 minutes

15. **[HEARTBEAT_MONITORING_DESIGN.md](HEARTBEAT_MONITORING_DESIGN.md)**
    - **Purpose**: Service health monitoring and heartbeat mechanisms
    - **Audience**: DevOps engineers, monitoring teams
    - **Key Content**: Health check design, heartbeat protocols, failure detection
    - **Connects To**: Production Monitoring → Health monitoring implementation
    - **Read Time**: 12 minutes

#### **🛡️ Resilience & Reliability**
16. **[RESILIENCE_PATTERNS.md](RESILIENCE_PATTERNS.md)**
    - **Purpose**: System resilience patterns and failure recovery strategies
    - **Audience**: System architects, reliability engineers
    - **Key Content**: Circuit breakers, retry patterns, graceful degradation
    - **Connects To**: Microservices Architecture → Reliability implementation
    - **Read Time**: 16 minutes

---

## 📋 **ARCHITECTURE DECISIONS**

### **The Decision Story: Why We Built It This Way**

#### **🎯 Formal Decision Records**
17. **[decisions/](decisions/)** - Architecture Decision Records (ADRs)
    - **[ADR-003-KIND-INTEGRATION-ENVIRONMENT.md](decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md)**
      - **Purpose**: Decision rationale for KIND integration environment
      - **Audience**: Developers, DevOps engineers
      - **Key Content**: Integration testing strategy, environment setup decisions
      - **Read Time**: 8 minutes

---

## 🗺️ **ARCHITECTURE READING PATHS**

### **For New Team Members**
1. Start: [Architecture Overview](KUBERNAUT_ARCHITECTURE_OVERVIEW.md)
2. Then: [Approved Microservices Architecture](APPROVED_MICROSERVICES_ARCHITECTURE.md)
3. Deep Dive: [Service Catalog](KUBERNAUT_SERVICE_CATALOG.md)
4. Implementation: [Implementation Roadmap](KUBERNAUT_IMPLEMENTATION_ROADMAP.md)

### **For Developers Building Services**
1. Start: [Service Catalog](KUBERNAUT_SERVICE_CATALOG.md)
2. Integration: [Microservices Communication](MICROSERVICES_COMMUNICATION_ARCHITECTURE.md)
3. Specific Service: Choose relevant architecture document
4. Operations: [Production Monitoring](PRODUCTION_MONITORING.md)

### **For AI/ML Engineers**
1. Start: Kubernaut Agent (KA) — `internal/kubernautagent/`, `cmd/kubernautagent/` (consolidated KA architecture doc set pending, tracked in Issue #1806)
2. Context: [AI Context Orchestration](AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md)
3. Intelligence: [Intelligence Pattern Discovery](INTELLIGENCE_PATTERN_DISCOVERY_ARCHITECTURE.md)
4. Toolsets: [Dynamic Toolset Configuration](DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md)

### **For DevOps/SRE Teams**
1. Start: [Production Monitoring](PRODUCTION_MONITORING.md)
2. Health: [Heartbeat Monitoring Design](HEARTBEAT_MONITORING_DESIGN.md)
3. Resilience: [Resilience Patterns](RESILIENCE_PATTERNS.md)
4. Network: [Service Connectivity Specification](SERVICE_CONNECTIVITY_SPECIFICATION.md)

---

## 📊 **DOCUMENTATION GOVERNANCE**

### **Authoritative Documents (Single Source of Truth)**
These 4 documents are the **ONLY** authoritative sources for architecture decisions:

1. **[KUBERNAUT_ARCHITECTURE_OVERVIEW.md](KUBERNAUT_ARCHITECTURE_OVERVIEW.md)** - V1 system design
2. **[KUBERNAUT_SERVICE_CATALOG.md](KUBERNAUT_SERVICE_CATALOG.md)** - V1 service specifications
3. **[KUBERNAUT_IMPLEMENTATION_ROADMAP.md](KUBERNAUT_IMPLEMENTATION_ROADMAP.md)** - V1/V2 strategy
4. **[APPROVED_MICROSERVICES_ARCHITECTURE.md](APPROVED_MICROSERVICES_ARCHITECTURE.md)** - V1 implementation specification

### **Document Status Legend**
- ✅ **CURRENT** - Use for development decisions
- ⚠️ **DEPRECATED** - Historical reference only, do not use
- 🔄 **DRAFT** - Work in progress, not for production use

### **Architecture Standards**
- **Port Assignments**: Context API (8091), Kubernaut Agent (8090) - standardized
- **Service Count**: V1.0 = 8 services, V1.1 = 9 services (+Effectiveness Monitor), V2.0 = 14+ services (+Dynamic Toolset, +others)
- **Architecture Version**: V1.0 is current implementation, V1.1 adds continuous improvement, V2 is future roadmap
- **Deferrals**: Dynamic Toolset→V2.0 (DD-016), Effectiveness Monitor→V1.1 (DD-017)

### **Related Documentation Directories**
- **Implementation**: [`../implementation/`](../implementation/) - Implementation-specific details and patterns
- **Strategic**: [`../strategic/`](../strategic/) - Strategic analysis and technology decisions
- **Concepts**: [`../concepts/`](../concepts/) - Conceptual clarifications and process flows
- **Deprecated**: [`../deprecated/architecture/`](../deprecated/architecture/) - Historical documents (14 files)

---

## 🎯 **QUICK REFERENCE**

### **V1 Architecture Metrics**
- **Services**: 10 core microservices with single responsibility
- **Timeline**: 3-4 weeks to production (95% confidence)
- **AI Integration**: Kubernaut Agent (KA) as primary investigation service
- **Performance**: <5s alert processing, 99.9% availability target
- **Business Value**: 40-60% faster MTTR, 20% operational cost reduction

### **Document Maintenance**
- All architecture changes must update the 4 authoritative documents
- New documents require approval and integration with existing structure
- Deprecated documents moved to [`../deprecated/architecture/`](../deprecated/architecture/)
- Regular reviews ensure documentation accuracy and relevance

---

*This comprehensive architecture documentation provides the complete blueprint for understanding, developing, and deploying Kubernaut's V1 intelligent Kubernetes remediation platform. Each document serves a specific purpose in the overall architectural narrative, from high-level vision to detailed implementation guidance.*