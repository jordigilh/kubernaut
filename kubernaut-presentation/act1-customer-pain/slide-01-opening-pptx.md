---
title: "Kubernaut"
subtitle: "Kubernetes AIOps Platform for Remediation"
author: "Kubernaut"
---

# Kubernaut

**Kubernetes AIOps Platform for Remediation**

*Kubernetes-native • Open Source • Production-ready*

::: notes
PM audience - frame as category-defining platform, not point solution.

Key positioning:
- "AIOps Platform" - not just automation, full platform
- "Kubernetes-native" - CRD-based, not external orchestration
- "Open Source" - Apache 2.0, community-driven
- "Production-ready" - Tekton Pipelines (CNCF graduated)

Opening: "Kubernaut is the first Kubernetes-native AIOps platform purpose-built for remediation."

Confidence: Strong technical foundation (11 microservices, 5 CRDs, Tekton integration)
:::

---

# The Market Opportunity

**The landscape:**

| **Category** | **Players** | **What They Do** | **The Gap** |
|--------------|-------------|------------------|-------------|
| **Observability** ($15B) | Datadog, Dynatrace, Splunk | Metrics, logs, traces | Tell you *what* broke |
| **Incident Mgmt** ($2B) | PagerDuty, Opsgenie | Alert routing, escalation | Tell you *who* to wake |
| **AIOps (Legacy)** | Moogsoft, BigPanda | Rule-based correlation | Brittle, not K8s-native |
| **→ Kubernaut** | *This space* | AI investigation + auto-fix | *Why* + *Fix* = **closed loop** |

**Market validation:** 96% K8s adoption [1], $5K-$9K/min downtime [2], 40% SRE toil [3]

*[See References slide for sources]*

::: notes
This is the competitive landscape slide - shows WHY there's an opportunity.

Walk through the table systematically:

**Row 1 - Observability ($15B market):**
"Datadog is a $2B+ ARR company, Dynatrace $1B+, Splunk part of Cisco"
"They're EXCELLENT at telling you WHAT broke"
"Metrics, logs, traces - best in class"
"But they stop at detection - remediation is manual"

**Row 2 - Incident Management ($2B market):**
"PagerDuty, Opsgenie - alert routing and escalation"
"They're EXCELLENT at telling you WHO to wake up"
"On-call schedules, escalation policies, notification delivery"
"But they don't analyze or fix - they just route alerts"

**Row 3 - Legacy AIOps (declining market):**
"Moogsoft, BigPanda - traditional 'AIOps' vendors"
"Promise: Intelligent alert correlation and root cause analysis"
"Reality: Rule-based systems that break with environment changes"
"KEY WEAKNESS: Not Kubernetes-native - external agents, no CRD integration"
"They failed because rules can't keep up with cloud-native complexity"

**Row 4 - Kubernaut (NEW category):**
"We're the ONLY player doing ALL of this:"
  1. AI investigation (not rules) - WHY did it break?
  2. Automated remediation - FIX it automatically
  3. Kubernetes-native - CRDs, not external agents
  4. Open source - Apache 2.0, transparent

The "closed loop" argument:
"Observability → Detection (WHAT)"
"Incident Mgmt → Routing (WHO)"
"Kubernaut → Analysis + Remediation (WHY + FIX)"

"We COMPLETE the loop that everyone else leaves open."

Key competitive advantages:
1. **vs Datadog/Dynatrace:**
   - They're metrics-first, we're remediation-first
   - They could add this, but remediation isn't their DNA
   - Customers want best-of-breed, not all-in-one

2. **vs PagerDuty/Opsgenie:**
   - They're routing-first, we're intelligence-first
   - They wake humans, we reduce the need to wake humans
   - Complementary - we integrate WITH them

3. **vs Moogsoft/BigPanda:**
   - They're rule-based, we're LLM-powered
   - They're external agents, we're K8s-native CRDs
   - They failed because rules break - AI adapts

4. **vs Hyperscalers (AWS/GCP/Azure):**
   - They're infrastructure-first, we're platform-first
   - They move slowly (18-36 month product cycles)
   - Multi-cloud is our advantage - they're single-cloud

Market validation data:
"96% K8s adoption - this isn't niche" [1]
"$5K-$9K/min downtime - massive pain" [2]
"40% SRE toil - unsustainable" [3]

The category creation pitch:
"This isn't 'better AIOps' - it's a NEW category"
"Kubernetes Remediation Platform"
"Like Terraform is for provisioning, Kubernaut is for remediation"

Exit scenarios (why this is attractive):
- Acquisition by Datadog/Dynatrace (add remediation to observability)
- Acquisition by hyperscaler (AWS/GCP/Azure want this capability)
- Independent growth (category leader, $100M+ ARR)

Transition: "Now let me show you the technical architecture that enables this..."
:::

---

# Product Architecture

**Kubernetes-native AIOps platform (11 microservices):**

**CRD Controllers (5):**
- RemediationOrchestrator → Central lifecycle
- RemediationProcessing → Signal enrichment
- AIAnalysis → HolmesGPT investigation
- WorkflowExecution → Tekton orchestration
- Notification → Multi-channel delivery

**Stateless Services (6):**
- Gateway, Context API, Data Storage, HolmesGPT-API, Dynamic Toolset, Effectiveness Monitor

**Execution Engine:** Tekton Pipelines (CNCF graduated)

::: notes
This is the technical credibility slide for PM audience.

Key differentiation points:
1. "Kubernetes-native" - Built ON Kubernetes, not just FOR Kubernetes
   - CRD-based architecture = native K8s semantics
   - No external orchestration required
   - Kubernetes API is the source of truth

2. "11 microservices" - This is a PLATFORM, not a tool
   - Each service has focused responsibility
   - Scales independently
   - Extensible architecture

3. "Tekton Pipelines" - Industry-standard execution
   - CNCF graduated project (like Kubernetes itself)
   - Enterprise trust and support
   - Not a proprietary execution engine

4. "HolmesGPT integration" - AI reasoning, not pattern matching
   - Open source AI investigation framework
   - Multi-LLM support (not vendor lock-in)
   - Community-driven improvements

Competitive positioning:
- vs Datadog/Dynatrace: We're remediation-first, they're metrics-first
- vs AIOps vendors: We're K8s-native CRDs, they're external agents
- vs DIY solutions: We're production-ready platform, not duct-tape scripts

Business model implications:
"Open source platform = community adoption + enterprise upsell"
"Kubernetes-native = low adoption friction for K8s teams"
"Tekton execution = enterprise confidence + security compliance"

Transition: "How does this architecture deliver value?"
:::

---

# How It Works: 5-Phase Pipeline

**Phase 1:** Gateway ingests alert (Prometheus, K8s Events, webhooks)
**Phase 2:** RemediationProcessing enriches with K8s context (~8KB)
**Phase 3:** AIAnalysis investigates with HolmesGPT (root cause + recommendations)
**Phase 4:** WorkflowExecution orchestrates Tekton PipelineRuns (29+ actions)
**Phase 5:** Notification delivers multi-channel updates (Slack, Teams, Email)

**Result:** Alert → Resolution in 2-5 minutes (vs. 45-60 minutes manual)

**29+ Remediation Actions:** Scaling, restarts, rollbacks, node ops, GitOps PRs

::: notes
This is the workflow credibility slide.

Walk through the technical sophistication:

Phase 1 - Gateway (Signal Intelligence):
"Not just alert ingestion - deduplication, classification, priority assignment"
"Redis-based fingerprinting prevents duplicate work"
"Rego policy engine for custom business logic"

Phase 2 - RemediationProcessing (Context Enrichment):
"~8KB of Kubernetes context per alert"
"Environment classification (prod/staging/dev)"
"Historical recovery context for repeated failures"
"This is what makes AI analysis accurate - rich context"

Phase 3 - AIAnalysis (HolmesGPT Investigation):
"Not generic ChatGPT - specialized K8s investigation framework"
"Multi-LLM support: OpenAI, Anthropic, local models"
"Confidence scoring: High (80%+), Medium (60-79%), Low (<60%)"
"Medium confidence triggers approval workflow"

Phase 4 - WorkflowExecution (Tekton Orchestration):
"Tekton Pipelines = CNCF graduated, enterprise-trusted"
"29+ remediation actions = comprehensive coverage"
"Parallel execution with dependency management"
"Cosign-signed container images for supply chain security"

Phase 5 - Notification (Multi-Channel):
"Slack, Teams, Email, SMS, webhooks"
"Rich context with action links (GitHub, Grafana, Prometheus)"
"Approval buttons for medium-confidence recommendations"

ROI calculation:
"50 incidents/month × 45 min saved × $200/hour = $7,500/month in engineer time"
"50 incidents/month × 45 min saved × $5K/hour downtime = $187K/month"
"Total value: ~$195K/month for medium enterprise"

Transition: "Why now? Why is this possible today and not 3 years ago?"
:::

---

# Why Now? Three Convergent Factors

**1. LLM Reasoning Breakthrough (2023+)**
- HolmesGPT: Open source K8s investigation framework
- Multi-LLM support (not vendor lock-in)
- Context understanding, not pattern matching

**2. Kubernetes Maturity**
- 96% adoption in enterprises
- CRD-based extensibility proven at scale
- Tekton Pipelines (CNCF graduated)

**3. Open Source AIOps Gap**
- Observability: Open source (Prometheus, Grafana)
- Remediation: Proprietary black boxes (until now)

**Kubernaut = First open source, K8s-native AIOps remediation platform**

::: notes
This is the category creation moment.

Why this timing is perfect:

Factor 1 - LLM Reasoning (2023+):
"Pre-2023: Rule-based automation failed because environments change"
"Post-GPT-4: LLMs can REASON about novel scenarios"
"HolmesGPT: Open source investigation framework (not proprietary AI)"
"Multi-LLM: OpenAI, Anthropic, local models - customer choice"

Factor 2 - Kubernetes Maturity:
"Kubernetes is 10 years old - production-proven"
"CRDs are THE extension pattern for K8s platforms"
"Tekton = CNCF graduated (same as Kubernetes, Prometheus, Envoy)"
"Enterprise customers trust CNCF projects"

Factor 3 - Open Source Gap:
"Observability stack is open: Prometheus, Grafana, Jaeger"
"But remediation? All proprietary: Dynatrace, Datadog, Moogsoft"
"This creates vendor lock-in and trust issues"
"Kubernaut: Apache 2.0 license = community-driven, transparent"

Category creation strategy:
"We're not competing in 'AIOps' - we're creating 'K8s Remediation Platform'"
"Like Terraform for provisioning, Kubernaut for remediation"
"Open source + enterprise support = proven GTM (Red Hat, HashiCorp model)"

Competitive moats from open source:
1. Community contributions accelerate feature development
2. Enterprise customers validate and harden the platform
3. Integration ecosystem grows organically
4. Security researchers find/fix vulnerabilities faster

First-mover advantages:
"We're 12-18 months ahead of competitors"
"Building open source community takes years - we start now"
"Enterprise trust in AIOps takes 24+ months - we're building it"

Transition: "What does this mean for go-to-market?"
:::

---

# Go-To-Market: Open Source + Enterprise

**Open Source (Apache 2.0):**
- Community adoption → Self-hosted deployment
- GitHub growth → Developer advocacy
- Integration ecosystem → Platform partnerships

**Enterprise Edition:**
- Multi-cluster support (10+ clusters)
- Advanced security (RBAC, audit, compliance)
- Enterprise support (SLA, TAM, training)
- Private LLM deployment

**Sales Motion:**
- **Land:** Open source community adoption
- **Expand:** Enterprise features for scale/security/support
- **Monetize:** Usage-based or seat-based pricing

::: notes
This is the open source business model slide.

The Open Source Strategy (Proven Pattern):
"Red Hat, HashiCorp, Confluent - all successful with this model"
"Open source = community-driven innovation"
"Enterprise = production-grade features for large orgs"

Phase 1 - Open Source Adoption:
"Free, self-hosted deployment"
"Single cluster, basic features"
"GitHub stars, community contributions"
"Developer advocacy: blogs, conference talks, tutorials"
"Target: 1K+ GitHub stars in Year 1"

Phase 2 - Enterprise Conversion:
"Multi-cluster support (Fortune 500 need this)"
"Advanced RBAC + compliance (SOC2, HIPAA, PCI)"
"Enterprise support: SLAs, dedicated TAM"
"Private LLM deployment (data sovereignty)"

Ideal Customer Profile:
"Open source: 100-1000 employees, 1-3 clusters"
"Enterprise: 1000-10,000 employees, 10+ clusters"
"Buying signal: 'We love the open source version, now we need...'"

Pricing Strategy:
"Open source: $0 (community support)"
"Enterprise: $50K-$500K/year based on:"
  - Number of clusters (tiered pricing)
  - Number of on-call engineers (seat-based)
  - Support tier (Standard, Premium, Elite)

Competitive advantages:
1. Open source = no vendor lock-in fear
2. K8s-native = low switching cost from DIY scripts
3. Tekton = enterprise trust (CNCF graduated)
4. Multi-LLM = customer choice (not forced to OpenAI)

GTM Channels:
"Community: GitHub, CNCF events, K8s meetups"
"Partners: Datadog, PagerDuty, AWS/GCP/Azure marketplaces"
"Direct: Enterprise sales for Fortune 1000"

Why this works:
"Open source builds trust and category leadership"
"Enterprise features have clear value (scale, security, support)"
"Conversion rate: 5-10% open source → enterprise (industry standard)"

Transition: "What does the revenue model look like?"
:::

---

# Revenue Model: Open Source Growth Path

**Year 1 - Community Building:**
- Open source GitHub launch
- 1K+ GitHub stars, 50-100 production deployments
- 5-10 design partners → Enterprise features
- **Goal:** Validation + initial revenue ($500K-$1M ARR)

**Year 2 - Enterprise Traction:**
- 100+ open source deployments
- 20-30 enterprise customers ($50K-$200K ACV)
- CNCF Sandbox acceptance
- **Goal:** $5-10M ARR, 10-15 enterprise customers

**Year 3 - Category Leadership:**
- 1000+ open source deployments
- 100+ enterprise customers ($100K-$500K ACV)
- CNCF Incubating, platform partnerships
- **Goal:** $30-50M ARR, Series B funding or profitability

::: notes
This is the realistic scale path for open source platforms.

Year 1 - Prove It Works:
"Open source first - build community trust"
"GitHub stars = leading indicator (1K stars = ~100 real users)"
"Design partners: 5-10 enterprises willing to pay for early features"
"Revenue: $500K-$1M (not the goal, proof of value is)"

Success metrics Year 1:
- GitHub stars: 1K+ (community interest)
- Production deployments: 50-100 (real usage)
- Customer renewal: 100% (they love it)
- Enterprise pipeline: 20+ companies interested

Year 2 - Scale Community + Revenue:
"100+ open source deployments = category validation"
"Enterprise customers: 20-30 at $50K-$200K ACV"
"CNCF Sandbox = enterprise credibility boost"
"Platform partnerships: Datadog, PagerDuty integrations"

Unit economics Year 2:
"CAC: $20K-$30K (open source reduces CAC)"
"ACV: $50K-$200K (multi-cluster, enterprise support)"
"LTV: $200K-$600K over 3 years (85%+ retention)"
"Gross margin: 80%+ (SaaS-like economics)"

Year 3 - Category Leader:
"1000+ deployments = clear category leader"
"100+ enterprise customers = predictable revenue"
"CNCF Incubating = production-ready stamp"
"$30-50M ARR = Series B or profitability"

Exit scenarios:
1. Acquisition by observability vendor ($200-400M, 5-8x revenue)
2. Acquisition by hyperscaler ($300-500M, strategic premium)
3. Independent growth → IPO path ($100M+ ARR)

Risks + Mitigation:
1. "Open source = no revenue?"
   - Mitigation: Red Hat model ($3B+ revenue), HashiCorp ($500M+ ARR)
   - Enterprise features have clear value at scale

2. "Hyperscalers will compete?"
   - Mitigation: They're slow (18-36 months), we're community-driven
   - Open source makes acquisition MORE likely

3. "LLM costs too high?"
   - Mitigation: Costs dropping 50% annually, local model support
   - Customers can BYO LLM (bring your own LLM)

4. "AI trust issues?"
   - Mitigation: Open source = transparent, auditab le
   - Always human-in-loop for risky actions

Competitive moat at scale:
"Network effects: Shared anonymized patterns across deployments"
"Community: 1000+ contributors by Year 3"
"Data: 100K+ incident resolutions = training data"
"Integrations: 50+ platform partners"

The Ask (for PM audience):
"We're building the Terraform of Kubernetes remediation"
"Open source + enterprise = proven path (Red Hat, HashiCorp, Confluent)"
"Early mover advantage in emerging category"
"Clear path to $50M+ ARR in 3 years"

Questions to ask audience:
"What concerns do you have about the model?"
"What would make this more compelling?"
"How does this compare to other platforms you've evaluated?"
:::

---

# References & Sources

**Market Statistics:**
1. **CNCF Survey 2023** - Kubernetes adoption rates
   [cncf.io/reports/cncf-annual-survey-2023](https://www.cncf.io/reports/cncf-annual-survey-2023/)

2. **Gartner / Atlassian** - Cost of downtime research
   [atlassian.com/incident-management/kpis/cost-per-incident](https://www.atlassian.com/incident-management/kpis/cost-per-incident)

3. **Google SRE Book** - Chapter 5: Eliminating Toil
   [sre.google/sre-book/eliminating-toil](https://sre.google/sre-book/eliminating-toil/)

4. **Datadog State of Monitoring** - Alert growth trends
   [datadoghq.com/state-of-monitoring](https://www.datadoghq.com/state-of-monitoring/)

**Technology References:**
- **Tekton Pipelines:** [tekton.dev](https://tekton.dev) (CNCF Graduated Project)
- **HolmesGPT:** [github.com/robusta-dev/holmesgpt](https://github.com/robusta-dev/holmesgpt) (Open Source)
- **Kubernaut:** [github.com/jordigilh/kubernaut](https://github.com/jordigilh/kubernaut) (Apache 2.0)

::: notes
Reference slide for credibility and fact-checking.

When presenting to PMs, having reference links shows:
1. Due diligence - you've done your homework
2. Transparency - claims are verifiable
3. Professionalism - not just marketing hype

Note: Some links are placeholders and should be verified:
- CNCF Survey: Check latest year available
- Gartner: Paid research, cite report name/year
- Atlassian: Public resource, verify URL

Keep this slide handy for Q&A - PMs may ask "where did you get that number?"

You can also create a one-pager handout with these references for follow-up.
:::


