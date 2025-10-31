# CRD Controller Gap Remediation Guide

**Purpose**: Step-by-step guide for using gap remediation templates
**Version**: 1.0
**Date**: 2025-10-22

---

> **📋 Design Decision: DD-006**
>
> **Controller Scaffolding Strategy**: Custom Production Templates (Approved)
> **See**: [DD-006-controller-scaffolding-strategy.md](../../architecture/decisions/DD-006-controller-scaffolding-strategy.md)
>
> These templates implement DD-006's approved scaffolding approach, chosen over Kubebuilder, Operator SDK, and manual creation for:
> - ✅ Automatic DD-005 Observability Standards enforcement
> - ✅ 40-60% time savings (4-6 hours per controller)
> - ✅ Consistency across all Kubernaut controllers
> - ✅ Centralized standards enforcement

---

## 📋 Overview

This guide walks you through using the gap remediation templates to rapidly implement production-ready CRD controllers. Following this process will save 40-60% implementation time while ensuring consistent quality across all controllers.

---

## 🎯 Who Should Use This Guide

- **Developers** implementing new CRD controllers
- **Platform Engineers** standardizing controller implementations
- **DevOps Engineers** deploying controllers to Kubernetes

---

## 📚 Prerequisites

Before starting, ensure you have:

1. ✅ Controller design complete (CRD schema, business logic defined)
2. ✅ Go development environment set up
3. ✅ Access to Kubernetes cluster for testing
4. ✅ Familiarity with controller-runtime framework
5. ✅ Understanding of your controller's specific requirements

---

## 🚀 Implementation Workflow

### Phase 1: Preparation (30 minutes)

#### Step 1.1: Define Controller Specifications

Create a specifications document with these details:

```yaml
controller_name: remediationprocessor        # Lowercase, no hyphens
controller_name_upper: REMEDIATIONPROCESSOR  # Uppercase
bin_name: remediation-processor              # Hyphenated binary name
image_name: remediationprocessor             # Container image name
package_path: github.com/jordigilh/kubernaut/pkg/remediationprocessor
crd_group: remediation.kubernaut.io
crd_version: v1alpha1
crd_kind: RemediationProcessing
crd_kind_lower: remediationprocessing
namespace: kubernaut-system
```

#### Step 1.2: Identify Controller-Specific Requirements

Document your controller's unique needs:

**Example for RemediationProcessor:**
```yaml
external_services:
  - name: PostgreSQL
    purpose: Data persistence
    config_fields:
      - postgres_host
      - postgres_port
      - postgres_user
      - postgres_password
  - name: Context API
    purpose: Historical context
    config_fields:
      - context_endpoint
      - context_timeout

custom_config:
  - section: classification
    fields:
      - semantic_threshold: float64
      - time_window_minutes: int
      - batch_size: int
```

#### Step 1.3: Review Template Library

Review all available templates:
- `cmd-main-template.go`
- `config-template.go`
- `config-test-template.go`
- `dockerfile-template`
- `configmap-template.yaml`
- `makefile-targets-template`
- `BUILD-template.md`
- `OPERATIONS-template.md`
- `DEPLOYMENT-template.md`

---

### Phase 2: Code Implementation (4-6 hours)

#### Step 2.1: Create Main Entry Point

```bash
# 1. Create directory
mkdir -p cmd/{{CONTROLLER_NAME}}

# 2. Copy and customize template
cp docs/templates/crd-controller-gap-remediation/cmd-main-template.go \
   cmd/{{CONTROLLER_NAME}}/main.go

# 3. Replace placeholders
sed -i '' 's/{{CONTROLLER_NAME}}/remediationprocessor/g' cmd/remediationprocessor/main.go
sed -i '' 's/{{PACKAGE_PATH}}/github.com\/jordigilh\/kubernaut\/pkg\/remediationprocessor/g' cmd/remediationprocessor/main.go
sed -i '' 's/{{CRD_GROUP}}/remediation.kubernaut.io/g' cmd/remediationprocessor/main.go
# ... continue for all placeholders

# 4. Uncomment TODOs and customize
# Open in editor and customize imports, reconciler setup, etc.
```

**Customization Checklist:**
- [ ] Replace all `{{PLACEHOLDER}}` values
- [ ] Uncomment CRD imports
- [ ] Uncomment config package imports
- [ ] Uncomment reconciler setup
- [ ] Add controller-specific initialization
- [ ] Verify imports resolve

#### Step 2.2: Create Config Package

```bash
# 1. Create directory
mkdir -p pkg/{{CONTROLLER_NAME}}/config

# 2. Copy templates
cp docs/templates/crd-controller-gap-remediation/config-template.go \
   pkg/{{CONTROLLER_NAME}}/config/config.go
cp docs/templates/crd-controller-gap-remediation/config-test-template.go \
   pkg/{{CONTROLLER_NAME}}/config/config_test.go

# 3. Replace placeholders
sed -i '' 's/{{CONTROLLER_NAME}}/remediationprocessor/g' pkg/remediationprocessor/config/*.go

# 4. Add controller-specific config structs
# Open config.go and add your custom configuration types
```

**Config Customization Example:**
```go
// In config.go, add after KubernetesConfig:

// DataStorageConfig holds configuration for the Data Storage Service.
type DataStorageConfig struct {
    PostgresHost     string `yaml:"postgres_host"`
    PostgresPort     int    `yaml:"postgres_port"`
    PostgresUser     string `yaml:"postgres_user"`
    PostgresPassword string `yaml:"postgres_password"`
    PostgresDatabase string `yaml:"postgres_database"`
    SSLMode          string `yaml:"ssl_mode"`
}

// Add to Config struct:
type Config struct {
    // ... existing fields ...
    DataStorage DataStorageConfig `yaml:"data_storage"`
}

// Add to setDefaults():
if c.DataStorage.PostgresPort == 0 {
    c.DataStorage.PostgresPort = 5432
}

// Add to Validate():
if c.DataStorage.PostgresHost == "" {
    return fmt.Errorf("data_storage.postgres_host is required")
}

// Add to LoadFromEnv():
if host := os.Getenv("POSTGRES_HOST"); host != "" {
    c.DataStorage.PostgresHost = host
}
```

**Config Test Customization:**
```go
// In config_test.go, add test cases:
{
    name: "missing postgres host",
    config: &config.Config{
        // ... common fields ...
        DataStorage: config.DataStorageConfig{
            PostgresHost: "", // Missing required field
        },
    },
    wantErr: true,
    errMsg:  "data_storage.postgres_host is required",
},
```

#### Step 2.3: Test Configuration

```bash
# Run config tests
go test -v ./pkg/{{CONTROLLER_NAME}}/config/...

# Expected output: All tests passing
```

**Validation Checklist:**
- [ ] Config loads from YAML file
- [ ] Defaults are set correctly
- [ ] Validation catches missing required fields
- [ ] Environment variables override config
- [ ] All tests pass

---

### Phase 3: Infrastructure (2-3 hours)

#### Step 3.1: Create Dockerfile

```bash
# 1. Copy template
cp docs/templates/crd-controller-gap-remediation/dockerfile-template \
   docker/{{IMAGE_NAME}}.Dockerfile

# 2. Replace placeholders
sed -i '' 's/{{CONTROLLER_NAME}}/remediationprocessor/g' docker/remediationprocessor.Dockerfile
sed -i '' 's/{{BIN_NAME}}/remediation-processor/g' docker/remediationprocessor.Dockerfile

# 3. Build test
podman build -f docker/remediationprocessor.Dockerfile -t test:latest .
```

#### Step 3.2: Create ConfigMap

```bash
# 1. Create directory
mkdir -p deploy/{{CONTROLLER_NAME}}

# 2. Copy template
cp docs/templates/crd-controller-gap-remediation/configmap-template.yaml \
   deploy/{{CONTROLLER_NAME}}/configmap.yaml

# 3. Customize with controller-specific config
# Open configmap.yaml and add your configuration
```

#### Step 3.3: Add Makefile Targets

```bash
# 1. Copy template
cp docs/templates/crd-controller-gap-remediation/makefile-targets-template \
   makefile-{{CONTROLLER_NAME}}-targets.txt

# 2. Replace placeholders
sed -i '' 's/{{CONTROLLER_NAME}}/remediationprocessor/g' makefile-remediationprocessor-targets.txt
sed -i '' 's/{{CONTROLLER_NAME_UPPER}}/REMEDIATIONPROCESSOR/g' makefile-remediationprocessor-targets.txt
sed -i '' 's/{{BIN_NAME}}/remediation-processor/g' makefile-remediationprocessor-targets.txt
sed -i '' 's/{{IMAGE_NAME}}/remediationprocessor/g' makefile-remediationprocessor-targets.txt

# 3. Append to main Makefile
cat makefile-remediationprocessor-targets.txt >> Makefile
rm makefile-remediationprocessor-targets.txt

# 4. Test Makefile targets
make remediationprocessor-build
make remediationprocessor-test
```

---

### Phase 4: Documentation (2-3 hours)

#### Step 4.1: Create BUILD.md

```bash
# 1. Create directory
mkdir -p docs/services/crd-controllers/XX-{{CONTROLLER_NAME}}

# 2. Copy template
cp docs/templates/crd-controller-gap-remediation/BUILD-template.md \
   docs/services/crd-controllers/XX-{{CONTROLLER_NAME}}/BUILD.md

# 3. Replace placeholders and customize
# Open BUILD.md and replace all {{PLACEHOLDER}} values
# Add controller-specific build instructions
```

#### Step 4.2: Create OPERATIONS.md

```bash
# 1. Copy template
cp docs/templates/crd-controller-gap-remediation/OPERATIONS-template.md \
   docs/services/crd-controllers/XX-{{CONTROLLER_NAME}}/OPERATIONS.md

# 2. Customize metrics and troubleshooting sections
# Add controller-specific operational procedures
```

#### Step 4.3: Create DEPLOYMENT.md

```bash
# 1. Copy template
cp docs/templates/crd-controller-gap-remediation/DEPLOYMENT-template.md \
   docs/services/crd-controllers/XX-{{CONTROLLER_NAME}}/DEPLOYMENT.md

# 2. Add deployment prerequisites and validation steps
# Document controller-specific deployment requirements
```

---

### Phase 5: Validation and Testing (2-4 hours)

#### Step 5.1: Comprehensive Validation

Run the validation checklist:

```bash
# Code Quality
[ ] No placeholder strings remain (grep -r "{{" pkg/{{CONTROLLER_NAME}}/ cmd/{{CONTROLLER_NAME}}/)
[ ] All imports resolve (go build ./cmd/{{CONTROLLER_NAME}})
[ ] Package names match directory structure
[ ] Code compiles without errors
[ ] All unit tests pass (make {{CONTROLLER_NAME}}-test)
[ ] Linting passes (make {{CONTROLLER_NAME}}-lint)
[ ] Code is formatted (make {{CONTROLLER_NAME}}-fmt)

# Infrastructure
[ ] Dockerfile builds successfully (make {{CONTROLLER_NAME}}-docker-build)
[ ] ConfigMap is valid Kubernetes YAML
[ ] All Makefile targets work
[ ] Container runs locally (make {{CONTROLLER_NAME}}-docker-run)

# Documentation
[ ] All template placeholders replaced
[ ] Controller-specific sections completed
[ ] Examples updated with real values
[ ] Cross-references are valid
[ ] TODOs addressed or documented
```

#### Step 5.2: Integration Testing

```bash
# 1. Deploy to test cluster
make {{CONTROLLER_NAME}}-deploy

# 2. Check health
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system

# 3. Test with sample resource
kubectl apply -f config/samples/{{CRD_GROUP}}_v1alpha1_{{CRD_KIND_LOWER}}.yaml

# 4. Verify reconciliation
kubectl get {{CRD_KIND}} -A
kubectl describe {{CRD_KIND}} <name> -n <namespace>
```

---

## 🔧 Troubleshooting Template Usage

### Common Issues

#### 1. Placeholder Not Replaced

**Symptom**: Build fails with "undefined: {{PLACEHOLDER}}"

**Solution**:
```bash
# Find remaining placeholders
grep -r "{{" pkg/{{CONTROLLER_NAME}}/ cmd/{{CONTROLLER_NAME}}/

# Replace manually or with sed
```

#### 2. Import Path Errors

**Symptom**: "package not found" errors

**Solution**:
- Verify go.mod module path
- Ensure directory structure matches package path
- Run `go mod tidy`

#### 3. Config Validation Failing

**Symptom**: Tests fail with "field is required"

**Solution**:
- Check test fixtures have all required fields
- Verify defaults are set in `setDefaults()`
- Ensure `Validate()` logic is correct

---

## 📊 Success Metrics

After completing gap remediation:

- ✅ **Build Success**: Code compiles without errors
- ✅ **Test Coverage**: >80% unit test coverage
- ✅ **Container Build**: Multi-arch image builds successfully
- ✅ **Deployment Success**: Controller runs in test cluster
- ✅ **Documentation Complete**: All 3 docs (BUILD, OPERATIONS, DEPLOYMENT) finished
- ✅ **Time Savings**: 40-60% faster than manual implementation

---

## 🎯 Best Practices

1. **Don't Skip Steps**: Follow the workflow sequentially
2. **Test Early**: Run tests after each phase
3. **Document as You Go**: Update docs with controller-specific details immediately
4. **Use Version Control**: Commit after each major step
5. **Review with Team**: Get code review before declaring complete

---

## 📚 Additional Resources

- [README.md](./README.md) - Template library overview
- [GAP_REMEDIATION_SESSION_SUMMARY.md](./GAP_REMEDIATION_SESSION_SUMMARY.md) - Implementation history
- [CRD_CONTROLLERS_GAP_TRIAGE.md](../../services/crd-controllers/CRD_CONTROLLERS_GAP_TRIAGE.md) - Gap analysis

---

**Document Status**: ✅ **PRODUCTION-READY**
**Last Updated**: 2025-10-22
**Maintained By**: Kubernaut Development Team
