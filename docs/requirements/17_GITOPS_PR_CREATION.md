# GitOps PR Creation - Business Requirements

**Document Version**: 1.0
**Date**: October 2025
**Status**: Business Requirements Specification - MVP Scope
**Module**: GitOps PR Creation (`pkg/gitops/`, AI Analysis Service integration)

---

## 1. Purpose & Scope

### 1.1 Business Purpose
The GitOps PR Creation component enables Kubernaut to automatically generate Git pull requests for declarative infrastructure changes, allowing AI-recommended remediations to follow GitOps best practices with human approval through Git workflows.

### 1.2 Scope - MVP (V1)
- **Git Repository Management**: Configuration and credential management for Git repositories
- **ArgoCD Integration**: Parse ArgoCD annotations to determine target Git files
- **Git Operations**: Clone, branch, commit, and push changes to Git repositories
- **GitHub API Integration**: Create pull requests via GitHub API with evidence-based justifications
- **Plain YAML Support**: Modify plain Kubernetes YAML manifests (no Helm/Kustomize in MVP)

### 1.3 Out of Scope - MVP (V1)
- ❌ GitLab support (Phase 2)
- ❌ Bitbucket support (Phase 2)
- ❌ Helm chart value resolution (Phase 2)
- ❌ Kustomize overlay patch generation (Phase 2)
- ❌ Advanced conflict resolution (Phase 2)
- ❌ PR status tracking with webhooks (Phase 2)

---

## 2. Core GitOps PR Creation

### 2.1 Business Capabilities

#### 2.1.1 GitOps Detection
- **BR-GITOPS-001**: MUST detect GitOps-managed resources and escalate with Git PR proposals
  - Detect ArgoCD annotations on Kubernetes resources
  - Generate evidence-based Git PR proposals with pattern analysis
  - Include AI confidence and justification in PR description
  - Support dual-track execution (immediate mitigation + Git PR)
  - **Integration**: Works with BR-AI-037 (pattern analysis) to ensure evidence-based PRs

#### 2.1.2 Git Repository Configuration
- **BR-GITOPS-002**: MUST support configurable Git repository mappings
  - ConfigMap or CRD for repository configuration
  - Repository URL, default branch, PR target branch specification
  - Path mappings from ArgoCD application name to Git manifest file path
  - Support for multiple Git repositories per environment
  - Per-environment repository configuration (production, staging, dev)
  - **MVP Scope**: Plain YAML manifests only
  - **Implementation**: ConfigMap in `kubernaut-system` namespace

**Configuration Example**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: git-repository-config
  namespace: kubernaut-system
data:
  repositories.yaml: |
    repositories:
      - name: "k8s-production-manifests"
        url: "https://github.com/company/k8s-manifests"
        provider: "github"  # MVP: github only
        defaultBranch: "main"
        prTargetBranch: "main"
        secretRef:
          name: "github-token"
          namespace: "kubernaut-system"
        pathMappings:
          - argocdApp: "webapp-prod"
            manifestPath: "production/webapp/deployment.yaml"
            type: "plain-yaml"  # MVP: plain-yaml only
          - argocdApp: "api-prod"
            manifestPath: "production/api/deployment.yaml"
            type: "plain-yaml"
```

#### 2.1.3 Git Credential Management
- **BR-GITOPS-003**: MUST support secure Git credential management
  - Store Git tokens/SSH keys in Kubernetes Secrets
  - Support GitHub Personal Access Token (PAT) authentication
  - Token rotation and expiration handling
  - RBAC for credential access (AI Analysis service only)
  - Secure credential injection for Git operations
  - Audit logging for credential usage
  - **Integration**: Extends BR-SEC-001 to BR-SEC-020 (general secrets management)

**Secret Example**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-token
  namespace: kubernaut-system
type: Opaque
stringData:
  token: "ghp_xxxxxxxxxxxxxxxxxxxx"  # GitHub Personal Access Token
  username: "kubernaut-bot"            # GitHub username (optional)
```

**RBAC Example**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: git-secret-reader
  namespace: kubernaut-system
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["github-token"]
  verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: aianalysis-git-secret-reader
  namespace: kubernaut-system
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
roleRef:
  kind: Role
  name: git-secret-reader
  apiGroup: rbac.authorization.k8s.io
```

---

### 2.2 ArgoCD Integration

#### 2.2.1 Annotation Parsing
- **BR-GITOPS-004**: MUST detect Git repository metadata from ArgoCD annotations
  - Parse `argocd.argoproj.io/tracking-id` annotation from Kubernetes resources
  - Extract repository name and file path from tracking-id
  - Support ArgoCD tracking-id formats: `{repo}:{path}` and `sha256:{hash}`
  - Fallback to ConfigMap path mappings if tracking-id unavailable
  - **MVP Scope**: Plain YAML files only (no Helm/Kustomize template resolution)

**Tracking-ID Format**:
```
argocd.argoproj.io/tracking-id: "company/k8s-manifests:production/webapp/deployment.yaml"
```

**Parsing Logic**:
```
1. Extract annotation value
2. Split by ':' → [repo_path, file_path]
3. Lookup repo_path in ConfigMap → full Git URL
4. Return: repository URL + file path
```

#### 2.2.2 AI File Detection
- **BR-GITOPS-005**: MUST use AI to determine target file for resource changes (MVP: Plain YAML only)
  - Analyze Kubernetes resource (Deployment, ConfigMap, etc.)
  - Determine which Git file controls the resource
  - Map resource field to YAML file path and field path
  - Provide confidence score for file detection (>0.8 required)
  - **MVP Limitation**: Plain YAML manifests only
  - **Phase 2**: Helm template resolution (`values.yaml` path mapping)
  - **Phase 2**: Kustomize overlay patch generation

**MVP Example (Plain YAML)**:
```
Resource: Deployment "webapp" in namespace "production"
Field to change: spec.template.spec.containers[0].resources.limits.memory

AI Analysis:
  - ArgoCD tracking-id: "company/k8s-manifests:production/webapp/deployment.yaml"
  - Target file: production/webapp/deployment.yaml
  - Target field path: spec.template.spec.containers[0].resources.limits.memory
  - Confidence: 0.95
  - Type: plain-yaml
```

---

### 2.3 Git Client Operations

#### 2.3.1 Git Repository Manipulation
- **BR-GITOPS-013**: MUST support Git client operations for repository manipulation
  - Clone remote Git repositories (HTTPS authentication)
  - Create branches from base branch
  - Modify files in working tree (plain YAML only for MVP)
  - Commit changes with proper author metadata
  - Push branches to remote repository
  - Handle Git authentication using token-based auth
  - Clean up temporary repositories after operations
  - Support shallow clones for large repositories (depth=1)
  - **Implementation**: Use `go-git/go-git/v5` library for pure Go implementation
  - **Container**: Include Git binary in container image for fallback

**Git Client Interface**:
```go
package gitops

import (
    "context"

    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing/transport/http"
)

type GitClient interface {
    // Clone clones a repository to a temporary directory
    Clone(ctx context.Context, url, branch string, auth *http.BasicAuth) (*git.Repository, string, error)

    // CreateBranch creates a new branch from current HEAD
    CreateBranch(repo *git.Repository, branchName string) error

    // ModifyFile modifies a file in the working tree
    ModifyFile(repoPath, filePath, newContent string) error

    // CommitAndPush commits changes and pushes to remote
    CommitAndPush(ctx context.Context, repo *git.Repository, message string, auth *http.BasicAuth) error

    // Cleanup removes temporary repository directory
    Cleanup(repoPath string) error
}
```

**Dependencies**:
```go
require github.com/go-git/go-git/v5 v5.11.0
```

---

### 2.4 GitHub API Integration

#### 2.4.1 Pull Request Creation
- **BR-GITOPS-014**: MUST support Git provider API operations for PR creation (MVP: GitHub only)
  - Create pull requests via GitHub REST API
  - Add reviewers to PRs based on environment/action configuration
  - Add labels to PRs for categorization (e.g., `kubernaut`, `remediation`, `oom`)
  - Set PR title and body with AI-generated evidence
  - Link to AIAnalysis CRD in PR description for audit trail
  - Handle API rate limiting with exponential backoff
  - **MVP Scope**: GitHub only
  - **Phase 2**: GitLab (Merge Requests), Bitbucket support
  - **Implementation**: Use `google/go-github/v57` for GitHub API

**GitHub API Client Interface**:
```go
package gitops

import (
    "context"

    "github.com/google/go-github/v57/github"
)

type GitHubClient interface {
    // CreatePR creates a pull request
    CreatePR(ctx context.Context, owner, repo string, opts CreatePROptions) (*PullRequest, error)

    // AddReviewers adds reviewers to an existing PR
    AddReviewers(ctx context.Context, owner, repo string, prNumber int, reviewers []string) error

    // AddLabels adds labels to an existing PR
    AddLabels(ctx context.Context, owner, repo string, prNumber int, labels []string) error
}

type CreatePROptions struct {
    Title      string
    Body       string
    HeadBranch string
    BaseBranch string
    Reviewers  []string
    Labels     []string
}

type PullRequest struct {
    Number    int
    URL       string
    CreatedAt string
}
```

**Dependencies**:
```go
require (
    github.com/google/go-github/v57 v57.0.0
    golang.org/x/oauth2 v0.15.0  // OAuth2 for GitHub
)
```

---

### 2.5 PR Template Configuration

#### 2.5.1 PR Content Templates
- **BR-GITOPS-006**: MUST support configurable PR templates
  - PR title, body, and metadata templates using Go templates
  - Automatic reviewer assignment based on environment (production, staging, dev)
  - Label assignment based on action type (oom, cpu, restart, etc.)
  - Link to AIAnalysis CRD for complete audit trail
  - Include pattern analysis evidence in PR body
  - Support for custom PR templates per repository

**Template Configuration**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: git-pr-templates
  namespace: kubernaut-system
data:
  default-pr-template: |
    ## 🤖 Kubernaut AI-Generated Remediation

<<<<<<< HEAD
    **Alert**: {{ .AlertName }}
=======
    **Alert**: {{ .SignalName }}
>>>>>>> crd_implementation
    **Environment**: {{ .Environment }}
    **Timestamp**: {{ .Timestamp }}

    ### 📊 Pattern Analysis Evidence (BR-AI-037)
    - **Event Frequency**: {{ .EventCount }} events in {{ .TimeWindow }}
    - **Resource Usage**: {{ .ResourceUsage }} ({{ .Trend }})
    - **Pattern Type**: {{ .PatternType }}
    - **AI Confidence**: {{ .Confidence }}

    ### 🔧 Proposed Change
    ```yaml
    {{ .DiffContent }}
    ```

    ### 📋 Business Requirement
    {{ .BusinessRequirement }}

    ### 🔗 Audit Trail
    - **AIAnalysis CR**: `kubectl get aianalysis {{ .AIAnalysisName }} -n {{ .Namespace }}`
    - **RemediationRequest CR**: `kubectl get alertremediation {{ .RemediationName }} -n {{ .Namespace }}`

    ### 👥 Reviewers
    {{ range .Reviewers }}
    - @{{ . }}
    {{ end }}

    ---
    *Auto-generated by Kubernaut AI Analysis (BR-AI-037, BR-GITOPS-001)*
    *Remediation ID: {{ .RemediationID }}*

  reviewer-mappings: |
    environments:
      production:
        reviewers: ["sre-team-lead", "platform-lead"]
        minApprovals: 2
      staging:
        reviewers: ["sre-oncall"]
        minApprovals: 1
      dev:
        reviewers: ["dev-team"]
        minApprovals: 1
```

---

### 2.6 Branch Strategy

#### 2.6.1 Branch Management
- **BR-GITOPS-007**: MUST support configurable branch strategies
  - Specify base branch (e.g., `main`, `master`, `production`)
  - Specify PR target branch (usually same as base)
  - PR branch naming conventions with template support
  - Branch prefix for Kubernaut-generated branches (e.g., `kubernaut/remediation-`)
  - Automatic branch cleanup after PR merge (optional)
  - Support for protected branches (fail gracefully with error message)

**Branch Strategy Configuration**:
```yaml
branchStrategy:
  baseBranch: "main"
  prTargetBranch: "main"
  prBranchPrefix: "kubernaut/remediation-"
  prBranchFormat: "{{ .Environment }}-{{ .ActionType }}-{{ .Timestamp }}"
  # Result: "kubernaut/remediation-production-oom-20250102-150430"

  autoCleanup: false  # Don't auto-delete branches (let Git provider handle it)
  allowProtectedBranches: false  # Fail if target branch is protected
```

---

## 3. Integration Points

### 3.1 AI Analysis Service Integration
- **BR-INT-AI-GITOPS-001**: AI Analysis service MUST integrate GitOps PR creation logic
  - Invoke GitOps client after determining declarative change is needed
  - Pass AIAnalysis context (alert data, pattern analysis, confidence) to GitOps client
  - Update AIAnalysis CRD status with Git PR reference
  - Handle GitOps failures gracefully (log error, update CRD status, escalate if needed)

**AIAnalysis CRD Status Update**:
```yaml
status:
  phase: "awaiting_git_pr"  # New phase for GitOps escalation
  gitPRReference:
    repository: "https://github.com/company/k8s-manifests"
    prNumber: 456
    prURL: "https://github.com/company/k8s-manifests/pull/456"
    createdAt: "2025-10-02T10:15:00Z"
    branch: "kubernaut/remediation-production-oom-20250102-150430"
```

### 3.2 RemediationRequest Integration
- **BR-INT-AR-GITOPS-001**: RemediationRequest MUST support GitOps escalation phase
  - Add `awaiting_git_pr` phase to RemediationRequest state machine
  - Track Git PR creation status in RemediationRequest status
  - Allow RemediationRequest to remain in `awaiting_git_pr` until PR is merged
  - **Phase 2**: Close RemediationRequest automatically when PR is merged + ArgoCD synced

---

## 4. MVP Limitations & Phase 2 Roadmap

### 4.1 MVP Limitations (Accepted for V1)
1. **Plain YAML Only**: No Helm chart value resolution or Kustomize overlay support
2. **GitHub Only**: No GitLab or Bitbucket support
3. **No Conflict Resolution**: Manual resolution required if duplicate PR exists
4. **No PR Status Tracking**: No automatic detection of PR merge/close
5. **No Multi-Repo Support**: Single repository per environment (workaround: multiple ConfigMap entries)

### 4.2 Phase 2 Enhancements (Future)
- **BR-GITOPS-008**: Mono-repo support with path-based access control
- **BR-GITOPS-009**: Helm chart value path resolution for Helm-managed resources
- **BR-GITOPS-010**: Kustomize overlay patch generation for Kustomize-managed resources
- **BR-GITOPS-011**: Concurrent PR conflict detection and resolution
- **BR-GITOPS-012**: PR status tracking with webhooks for automatic RemediationRequest closure
- **BR-GITOPS-014-EXTENDED**: GitLab (Merge Requests) and Bitbucket support

---

## 5. Performance Requirements

### 5.1 Git Operations Performance
- **BR-PERF-GITOPS-001**: Git clone operations MUST complete within 30 seconds for typical repositories (<100MB)
- **BR-PERF-GITOPS-002**: PR creation MUST complete within 10 seconds after branch push
- **BR-PERF-GITOPS-003**: MUST support shallow clones (depth=1) for large repositories

### 5.2 Scalability
- **BR-SCALE-GITOPS-001**: MUST support up to 50 concurrent Git operations (clone + commit + push)
- **BR-SCALE-GITOPS-002**: MUST handle GitHub API rate limits (5000 requests/hour for authenticated requests)

---

## 6. Security Requirements

### 6.1 Credential Security
- **BR-SEC-GITOPS-001**: MUST store Git credentials in Kubernetes Secrets only
- **BR-SEC-GITOPS-002**: MUST never log Git credentials (tokens, SSH keys)
- **BR-SEC-GITOPS-003**: MUST use RBAC to restrict Git credential access to AI Analysis service only
- **BR-SEC-GITOPS-004**: MUST clean up Git credentials from memory after use

### 6.2 Git Repository Security
- **BR-SEC-GITOPS-005**: MUST validate Git repository URLs before cloning (whitelist approach)
- **BR-SEC-GITOPS-006**: MUST use HTTPS for Git operations (no SSH for MVP)
- **BR-SEC-GITOPS-007**: MUST verify Git repository ownership before creating PR

---

## 7. Monitoring & Observability

### 7.1 Prometheus Metrics
- **BR-METRICS-GITOPS-001**: MUST expose Prometheus metrics for Git operations
  - `gitops_clone_duration_seconds` (histogram)
  - `gitops_commit_push_duration_seconds` (histogram)
  - `gitops_pr_creation_duration_seconds` (histogram)
  - `gitops_pr_created_total` (counter, labels: repository, environment)
  - `gitops_pr_creation_errors_total` (counter, labels: repository, error_type)

### 7.2 Logging
- **BR-LOG-GITOPS-001**: MUST log all Git operations with structured logging
  - Repository URL (redacted credentials), branch name, file path, commit SHA
- **BR-LOG-GITOPS-002**: MUST log PR creation with PR URL and PR number

---

## 8. Error Handling

### 8.1 Git Operation Failures
- **BR-ERROR-GITOPS-001**: MUST handle Git authentication failures gracefully
  - Log error with specific failure reason (invalid token, expired token, etc.)
  - Update AIAnalysis CRD status with error message
  - Do NOT retry on authentication failures (manual intervention required)

- **BR-ERROR-GITOPS-002**: MUST handle Git clone failures with retry logic
  - Retry up to 3 times with exponential backoff (1s, 2s, 4s)
  - Log each retry attempt with failure reason

- **BR-ERROR-GITOPS-003**: MUST handle PR creation failures gracefully
  - Check for existing PR with same head branch (GitHub API)
  - If exists, update existing PR comment instead of failing
  - Log PR creation failure with specific GitHub API error

---

## 9. Success Criteria

### 9.1 MVP Success Metrics
- ✅ AI Analysis service can create Git PRs for plain YAML manifests
- ✅ Git PRs include evidence-based justification from pattern analysis (BR-AI-037)
- ✅ PRs are created in GitHub with correct title, body, reviewers, and labels
- ✅ Git operations complete within performance targets (<30s clone, <10s PR)
- ✅ Credentials are securely managed via Kubernetes Secrets
- ✅ All Git operations are logged and metrics are exposed

### 9.2 Phase 2 Success Metrics (Future)
- Support for Helm chart value path resolution (80% of GitOps deployments)
- Support for Kustomize overlay patch generation
- GitLab and Bitbucket support
- PR status tracking with automatic RemediationRequest closure

---

## 10. Dependencies

### 10.1 Go Library Dependencies
```go
require (
    github.com/go-git/go-git/v5 v5.11.0           // Git client operations
    github.com/google/go-github/v57 v57.0.0       // GitHub API (MVP)
    golang.org/x/oauth2 v0.15.0                   // OAuth2 for GitHub
)
```

### 10.2 Container Image Requirements
```dockerfile
FROM alpine:3.19
RUN apk add --no-cache git ca-certificates  # Git binary for fallback
COPY aianalysis /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/aianalysis"]
```

### 10.3 Kubernetes Resources
- ConfigMap: `git-repository-config` (repository configuration)
- ConfigMap: `git-pr-templates` (PR templates and reviewer mappings)
- Secret: `github-token` (GitHub Personal Access Token)
- RBAC: Role + RoleBinding for Secret access

---

## 11. Implementation Timeline (MVP)

### 11.1 MVP Implementation Effort
**Total**: 20-32 days (4-6.5 weeks)

| Component | Effort | Priority |
|---|---|---|
| **BR-GITOPS-002**: Repository configuration (ConfigMap) | 2-3 days | CRITICAL |
| **BR-GITOPS-003**: Git credential management (Secret + RBAC) | 1-2 days | CRITICAL |
| **BR-GITOPS-004**: ArgoCD annotation parsing | 3-5 days | CRITICAL |
| **BR-GITOPS-005**: AI file detection (plain YAML) | 3-5 days | CRITICAL |
| **BR-GITOPS-006**: PR template configuration | 2-3 days | HIGH |
| **BR-GITOPS-007**: Branch strategy | 1-2 days | HIGH |
| **BR-GITOPS-013**: Git client library integration | 3-5 days | CRITICAL |
| **BR-GITOPS-014**: GitHub API integration | 4-6 days | CRITICAL |
| Integration testing | 3-4 days | HIGH |

---

## 12. Approval Record

**Approved By**: User
**Approval Date**: 2025-10-02
**Approved Scope**: MVP (Plain YAML + GitHub only)
**Approved Libraries**:
- `go-git/go-git/v5` for Git operations
- `google/go-github/v57` for GitHub API
- GitLab support deferred to Phase 2 (use `xanzy/go-gitlab` when implementing)

**Phase 2 Deferred**:
- GitLab support (use `xanzy/go-gitlab` - de facto standard)
- Bitbucket support
- Helm/Kustomize support
- Conflict resolution
- PR status tracking

---

**Document Status**: ✅ **APPROVED FOR MVP IMPLEMENTATION**

