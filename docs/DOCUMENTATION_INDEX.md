# Kubernaut Documentation Index

## Quick Navigation

This index provides comprehensive navigation across all Kubernaut documentation with cross-references between business requirements, architecture, and implementation guides.

---

## 📋 **Business Requirements** (1,452+ Requirements)

### Core Requirements Modules
| Module | Requirements | Key Components | Architecture Links |
|--------|-------------|----------------|-------------------|
| **[01_MAIN_APPLICATIONS](requirements/01_MAIN_APPLICATIONS.md)** | 127 | Alert Processing, AI Decision Making | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) |
| **[02_AI_MACHINE_LEARNING](requirements/02_AI_MACHINE_LEARNING.md)** | 185 | Pattern Recognition, ML Analytics | [Intelligence & Pattern Discovery](architecture/INTELLIGENCE_PATTERN_DISCOVERY_ARCHITECTURE.md) |
| **[03_PLATFORM_KUBERNETES_OPERATIONS](requirements/03_PLATFORM_KUBERNETES_OPERATIONS.md)** | 142 | K8s Operations, Resource Management | [Workflow Engine](architecture/WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md) |
| **[04_WORKFLOW_ENGINE_ORCHESTRATION](requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md)** | 165 | Adaptive Orchestration, Step Execution | [Workflow Engine](architecture/WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md) |
| **[05_STORAGE_DATA_MANAGEMENT](requirements/05_STORAGE_DATA_MANAGEMENT.md)** | 135 | Multi-modal Storage, Caching | [Storage & Data Management](architecture/STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md) |
| **[06_INTEGRATION_LAYER](requirements/06_INTEGRATION_LAYER.md)** | 128 | API Integration, External Services | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) |
| **[07_INTELLIGENCE_PATTERN_DISCOVERY](requirements/07_INTELLIGENCE_PATTERN_DISCOVERY.md)** | 150 | Anomaly Detection, Clustering | [Intelligence & Pattern Discovery](architecture/INTELLIGENCE_PATTERN_DISCOVERY_ARCHITECTURE.md) |
| **[08_INFRASTRUCTURE_MONITORING](requirements/08_INFRASTRUCTURE_MONITORING.md)** | 98 | Health Monitoring, Metrics | [Enhanced Health Monitoring](requirements/14_ENHANCED_HEALTH_MONITORING.md) |
| **[09_SHARED_UTILITIES_COMMON](requirements/09_SHARED_UTILITIES_COMMON.md)** | 112 | Common Services, Utilities | Multiple Architecture Documents |
| **[10_AI_CONTEXT_ORCHESTRATION](requirements/10_AI_CONTEXT_ORCHESTRATION.md)** | 180 | Dynamic Context, Performance | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) |
| **[11_SECURITY_ACCESS_CONTROL](requirements/11_SECURITY_ACCESS_CONTROL.md)** | 85 | RBAC, Authentication | Security sections in all architectures |
| **[12_API_SERVER_SERVICES](requirements/12_API_SERVER_SERVICES.md)** | 92 | REST APIs, Service Integration | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) |
| **[13_HOLMESGPT_REST_API_WRAPPER](requirements/13_HOLMESGPT_REST_API_WRAPPER.md)** | 118 | HolmesGPT Integration | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) |
| **[13_INFRASTRUCTURE_PLATFORM](requirements/13_INFRASTRUCTURE_PLATFORM.md)** | 95 | Platform Services | [Storage & Data Management](architecture/STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md) |
| **[14_ENHANCED_HEALTH_MONITORING](requirements/14_ENHANCED_HEALTH_MONITORING.md)** | 65 | Advanced Health Checks | Health sections in all architectures |

---

## 🏗️ **System Architecture**

### Comprehensive Architecture Documents
| Architecture | Business Requirements Coverage | Key Features | Related Requirements |
|-------------|-------------------------------|--------------|---------------------|
| **[AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md)** | 180 Requirements (BR-CONTEXT-001 to BR-CONTEXT-043) | Dynamic Context Discovery, Intelligent Caching, Performance Optimization | [10_AI_CONTEXT_ORCHESTRATION](requirements/10_AI_CONTEXT_ORCHESTRATION.md), [13_HOLMESGPT_REST_API_WRAPPER](requirements/13_HOLMESGPT_REST_API_WRAPPER.md) |
| **[Intelligence & Pattern Discovery](architecture/INTELLIGENCE_PATTERN_DISCOVERY_ARCHITECTURE.md)** | 150 Requirements (BR-INTELLIGENCE-001 to BR-INTELLIGENCE-150) | Pattern Recognition, Anomaly Detection, ML Analytics | [02_AI_MACHINE_LEARNING](requirements/02_AI_MACHINE_LEARNING.md), [07_INTELLIGENCE_PATTERN_DISCOVERY](requirements/07_INTELLIGENCE_PATTERN_DISCOVERY.md) |
| **[Workflow Engine & Orchestration](architecture/WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md)** | 165 Requirements (BR-WORKFLOW-001 to BR-AUTOMATION-030) | Adaptive Orchestration, Step Execution, State Management | [04_WORKFLOW_ENGINE_ORCHESTRATION](requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md), [03_PLATFORM_KUBERNETES_OPERATIONS](requirements/03_PLATFORM_KUBERNETES_OPERATIONS.md) |
| **[Storage & Data Management](architecture/STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md)** | 135 Requirements (BR-STORAGE-001 to BR-PERSISTENCE-015) | Multi-modal Storage, Vector DB, Intelligent Caching | [05_STORAGE_DATA_MANAGEMENT](requirements/05_STORAGE_DATA_MANAGEMENT.md), [13_INFRASTRUCTURE_PLATFORM](requirements/13_INFRASTRUCTURE_PLATFORM.md) |

### Legacy Architecture Documents (Archived)
- **[archived_legacy/ARCHITECTURE.md](archived_legacy/ARCHITECTURE.md)** - Superseded by comprehensive architecture documents
- **[archived_legacy/HOLMESGPT_INTEGRATION.md](archived_legacy/HOLMESGPT_INTEGRATION.md)** - Superseded by AI Context Orchestration
- **[archived_legacy/WORKFLOWS.md](archived_legacy/WORKFLOWS.md)** - Superseded by Workflow Engine & Orchestration

---

## 🔧 **Development & Implementation**

### Implementation Guides
| Guide | Focus Area | Related Architecture | Business Requirements |
|-------|------------|---------------------|---------------------|
| **[TESTING_FRAMEWORK.md](TESTING_FRAMEWORK.md)** | Testing Strategy | All Architectures | Quality requirements across all modules |
| **[development/project guidelines.md](development/project%20guidelines.md)** | Development Standards | All Architectures | Development process requirements |
| **[development/HOLMESGPT_DEPLOYMENT.md](development/HOLMESGPT_DEPLOYMENT.md)** | HolmesGPT Integration | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) | [13_HOLMESGPT_REST_API_WRAPPER](requirements/13_HOLMESGPT_REST_API_WRAPPER.md) |
| **[development/LLM_CONTEXT_ENRICHMENT_GUIDE.md](development/LLM_CONTEXT_ENRICHMENT_GUIDE.md)** | Context Enhancement | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) | [10_AI_CONTEXT_ORCHESTRATION](requirements/10_AI_CONTEXT_ORCHESTRATION.md) |

### Testing Documentation
| Test Category | Documentation | Coverage | Related Requirements |
|--------------|---------------|----------|---------------------|
| **Integration Testing** | [test/integration/](test/integration/) | End-to-end workflows | All business requirements |
| **Unit Testing** | [test/unit/](test/unit/) | Component testing | Module-specific requirements |
| **E2E Testing** | [test/e2e/](test/e2e/) | Complete scenarios | [01_MAIN_APPLICATIONS](requirements/01_MAIN_APPLICATIONS.md) |
| **Business Requirements Testing** | [requirements/tests/](requirements/tests/) | Requirements validation | All business requirements |

---

## 📊 **Project Status & Planning**

### Current Status
| Document | Purpose | Last Updated | Key Metrics |
|----------|---------|-------------|-------------|
| **[status/PROJECT_STATUS_CONSOLIDATED.md](status/PROJECT_STATUS_CONSOLIDATED.md)** | Comprehensive Status | September 2025 | 85% Milestone 1 Complete, 1,452 Requirements |
| **[requirements/00_REQUIREMENTS_OVERVIEW.md](requirements/00_REQUIREMENTS_OVERVIEW.md)** | Requirements Summary | September 2025 | 16 Modules, 1,452 Requirements |

### Planning & Analysis
| Document | Focus | Related Architecture |
|----------|-------|---------------------|
| **[planning/](planning/)** | Project Planning | All Architectures |
| **[analysis/](analysis/)** | System Analysis | Architecture Dependencies |

---

## 🚀 **Deployment & Operations**

### Deployment Guides
| Environment | Documentation | Requirements Addressed |
|------------|---------------|----------------------|
| **Development** | [getting-started/](getting-started/) | Development environment setup |
| **Integration** | [deployment/](deployment/) | Integration testing setup |
| **Production** | [deployment/MILESTONE_1_CONFIGURATION_OPTIONS.md](deployment/MILESTONE_1_CONFIGURATION_OPTIONS.md) | Production deployment |

### Health Monitoring
| Component | Monitoring Guide | Architecture Reference |
|-----------|-----------------|----------------------|
| **Enhanced Health** | [development/HEARTBEAT_INTEGRATION_GUIDE.md](development/HEARTBEAT_INTEGRATION_GUIDE.md) | Health sections in all architectures |
| **System Metrics** | [requirements/14_ENHANCED_HEALTH_MONITORING.md](requirements/14_ENHANCED_HEALTH_MONITORING.md) | [Enhanced Health Monitoring](requirements/14_ENHANCED_HEALTH_MONITORING.md) |

---

## 📚 **Specialized Documentation**

### HolmesGPT Integration
| Document | Purpose | Architecture Link |
|----------|---------|------------------|
| **[holmesgpt/DYNAMIC_TOOLSET_CONFIGURATION.md](holmesgpt/DYNAMIC_TOOLSET_CONFIGURATION.md)** | Toolset Management | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) |
| **[holmesgpt/INTEGRATION_IMPLEMENTATION_COMPLETE.md](holmesgpt/INTEGRATION_IMPLEMENTATION_COMPLETE.md)** | Implementation Status | [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md) |

### Vector Database
| Document | Purpose | Architecture Link |
|----------|---------|------------------|
| **[VECTOR_DATABASE_SELECTION.md](VECTOR_DATABASE_SELECTION.md)** | Vector DB Analysis | [Storage & Data Management](architecture/STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md) |

---

## 🔗 **Cross-Reference Quick Links**

### By Business Requirement Categories
- **Alert Processing**: [01_MAIN_APPLICATIONS](requirements/01_MAIN_APPLICATIONS.md) → [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md)
- **AI & ML**: [02_AI_MACHINE_LEARNING](requirements/02_AI_MACHINE_LEARNING.md) → [Intelligence & Pattern Discovery](architecture/INTELLIGENCE_PATTERN_DISCOVERY_ARCHITECTURE.md)
- **Kubernetes Operations**: [03_PLATFORM_KUBERNETES_OPERATIONS](requirements/03_PLATFORM_KUBERNETES_OPERATIONS.md) → [Workflow Engine](architecture/WORKFLOW_ENGINE_ORCHESTRATION_ARCHITECTURE.md)
- **Data Storage**: [05_STORAGE_DATA_MANAGEMENT](requirements/05_STORAGE_DATA_MANAGEMENT.md) → [Storage & Data Management](architecture/STORAGE_DATA_MANAGEMENT_ARCHITECTURE.md)
- **Context Orchestration**: [10_AI_CONTEXT_ORCHESTRATION](requirements/10_AI_CONTEXT_ORCHESTRATION.md) → [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md)

### By Implementation Priority
1. **Production Ready**: AI Context Orchestration, Security Framework, State Storage
2. **Integration Testing**: Workflow Engine, Pattern Discovery, Storage Management
3. **Enhancement Phase**: Advanced ML Analytics, Cross-cluster Operations

---

## 📖 **Documentation Guidelines**

- **Business Requirements**: Start with [requirements/00_REQUIREMENTS_OVERVIEW.md](requirements/00_REQUIREMENTS_OVERVIEW.md)
- **Architecture Understanding**: Begin with [AI Context Orchestration](architecture/AI_CONTEXT_ORCHESTRATION_ARCHITECTURE.md)
- **Implementation**: Follow [development/project guidelines.md](development/project%20guidelines.md)
- **Testing**: Reference [TESTING_FRAMEWORK.md](TESTING_FRAMEWORK.md)
- **Deployment**: Start with [getting-started/](getting-started/)

---

*This index is maintained as part of the documentation reorganization initiative to ensure comprehensive cross-referencing between business requirements, architecture, and implementation documentation.*