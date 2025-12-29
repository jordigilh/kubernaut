# Shared Infrastructure Abstraction Proposal

**Date**: December 22, 2025  
**Status**: 💡 **PROPOSAL**  
**Goal**: Abstract DS bootstrap + Generic container orchestration for all services  
**Confidence**: 95%

---

## 🎯 **Proposal Summary**

Create two abstraction layers in `test/infrastructure/`:

1. **Opinionated DataStorage Bootstrap** (`datastorage_bootstrap.go`) ✅ **ALREADY EXISTS**
   - Standardized PostgreSQL + Redis + DataStorage startup
   - Configurable ports per service
   - Sequential DD-TEST-002 pattern
   - Eliminates 95%+ code duplication

2. **Generic Container Orchestration** (`container.go`) 🆕 **NEW**
   - Build and start any container (e.g., HolmesGPT API, Embedding Service)
   - Health check abstraction
   - Network management
   - Reusable by all services

---

## 📋 **Part 1: Opinionated DataStorage Bootstrap (EXISTING)**

### **Current Implementation**

**File**: `test/infrastructure/datastorage_bootstrap.go`

**Already Provides**:
```go
type DSBootstrapConfig struct {
    ServiceName string // e.g., "aianalysis", "gateway"
    
    // Ports (per DD-TEST-001)
    PostgresPort    int // 15438
    RedisPort       int // 16384
    DataStoragePort int // 18095
    MetricsPort     int // 19095
    
    // Directories
    MigrationsDir string // "migrations"
    ConfigDir     string // "test/integration/{service}/config"
    
    // Database
    PostgresUser     string
    PostgresPassword string
    PostgresDB       string
}

func StartDSBootstrap(cfg DSBootstrapConfig, writer io.Writer) (*DSBootstrapInfra, error)
func StopDSBootstrap(infra *DSBootstrapInfra, writer io.Writer) error
```

### **Enhancement: Add Service-Specific Overrides**

```go
type DSBootstrapConfig struct {
    // ... existing fields ...
    
    // NEW: Optional overrides
    PostgresImage    string // Default: "postgres:16-alpine"
    RedisImage       string // Default: "redis:7-alpine"
    DataStorageImage string // Default: "localhost/kubernaut-datastorage:latest"
    
    // NEW: Optional environment variables for DataStorage
    DataStorageEnv map[string]string // e.g., {"MOCK_LLM_MODE": "true"}
    
    // NEW: Skip components (for services that don't need all of DS stack)
    SkipRedis bool // Gateway doesn't need Redis directly
}
```

---

## 📋 **Part 2: Generic Container Orchestration (NEW)**

### **Proposed API**

**File**: `test/infrastructure/container.go` 🆕

```go
package infrastructure

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "os/exec"
    "time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Generic Container Orchestration
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Provides reusable abstractions for building and running containers in integration tests.
// Eliminates code duplication across services (AIAnalysis, Gateway, etc.)
//
// Design Principles:
// - Declarative configuration (ContainerConfig)
// - Sequential startup with health checks (DD-TEST-002)
// - Port management (DD-TEST-001)
// - Network-aware (automatic network creation)
//
// Usage:
//   cfg := ContainerConfig{
//       Name: "aianalysis_holmesgpt_1",
//       Image: "localhost/kubernaut-holmesgpt-api:latest",
//       BuildContext: ContainerBuildConfig{
//           Dockerfile: "holmesgpt-api/Dockerfile",
//           Tag: "localhost/kubernaut-holmesgpt-api:latest",
//       },
//       Ports: map[int]int{18120: 8080}, // host:container
//       Env: map[string]string{
//           "MOCK_LLM_MODE": "true",
//           "DATASTORAGE_URL": "http://datastorage:8080",
//       },
//       Network: "aianalysis_test-network",
//       HealthCheck: HealthCheckConfig{
//           URL: "http://localhost:18120/health",
//           Timeout: 60 * time.Second,
//       },
//   }
//   
//   container, err := StartContainer(cfg, writer)
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ContainerConfig defines configuration for starting a container
type ContainerConfig struct {
    // Name is the container name (required)
    Name string
    
    // Image is the container image (required)
    Image string
    
    // BuildContext defines how to build the image (optional)
    // If provided, image will be built before starting
    BuildContext *ContainerBuildConfig
    
    // Ports maps host ports to container ports
    // Example: map[int]int{18120: 8080} means host:18120 -> container:8080
    Ports map[int]int
    
    // Env provides environment variables
    Env map[string]string
    
    // Network is the container network (optional, will create if doesn't exist)
    Network string
    
    // Volumes maps host paths to container paths (optional)
    // Example: map[string]string{"/host/path": "/container/path:ro"}
    Volumes map[string]string
    
    // HealthCheck defines how to validate container health (optional)
    HealthCheck *HealthCheckConfig
    
    // DependsOn lists container names that must be healthy before starting (optional)
    DependsOn []string
}

// ContainerBuildConfig defines how to build a container image
type ContainerBuildConfig struct {
    // Dockerfile path relative to ContextDir
    Dockerfile string
    
    // ContextDir is the build context directory (defaults to project root)
    ContextDir string
    
    // Tag is the image tag (e.g., "localhost/kubernaut-holmesgpt-api:latest")
    Tag string
    
    // Args provides build arguments (optional)
    Args map[string]string
}

// HealthCheckConfig defines container health validation
type HealthCheckConfig struct {
    // Type of health check ("http", "tcp", "exec")
    Type string // Default: "http" if URL provided, "tcp" if Port provided
    
    // URL for HTTP health checks (e.g., "http://localhost:18120/health")
    URL string
    
    // Port for TCP health checks (e.g., 5432)
    Port int
    
    // ExecCommand for exec health checks (e.g., ["pg_isready", "-U", "slm_user"])
    ExecCommand []string
    
    // Timeout for health check to succeed
    Timeout time.Duration
    
    // Interval between health check attempts
    Interval time.Duration // Default: 1s
    
    // ExpectedStatus for HTTP checks (default: 200)
    ExpectedStatus int
}

// Container represents a running container
type Container struct {
    Name    string
    Image   string
    ID      string // Container ID from podman
    Network string
    Ports   map[int]int
}

// StartContainer builds (if needed) and starts a container with health checks
//
// Returns:
// - *Container: Running container metadata
// - error: Any errors during build/start/health check
func StartContainer(cfg ContainerConfig, writer io.Writer) (*Container, error) {
    fmt.Fprintf(writer, "🚀 Starting container: %s\n", cfg.Name)
    
    // 1. Build image if BuildContext provided
    if cfg.BuildContext != nil {
        fmt.Fprintf(writer, "   🔨 Building image: %s\n", cfg.BuildContext.Tag)
        if err := buildImage(cfg.BuildContext, writer); err != nil {
            return nil, fmt.Errorf("failed to build image: %w", err)
        }
    }
    
    // 2. Create network if needed
    if cfg.Network != "" {
        if err := ensureNetwork(cfg.Network, writer); err != nil {
            return nil, fmt.Errorf("failed to ensure network: %w", err)
        }
    }
    
    // 3. Stop and remove existing container
    cleanup(cfg.Name, writer)
    
    // 4. Build podman run command
    args := []string{"run", "-d", "--name", cfg.Name}
    
    if cfg.Network != "" {
        args = append(args, "--network", cfg.Network)
    }
    
    for hostPort, containerPort := range cfg.Ports {
        args = append(args, "-p", fmt.Sprintf("%d:%d", hostPort, containerPort))
    }
    
    for key, value := range cfg.Env {
        args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
    }
    
    for hostPath, containerPath := range cfg.Volumes {
        args = append(args, "-v", fmt.Sprintf("%s:%s", hostPath, containerPath))
    }
    
    args = append(args, cfg.Image)
    
    // 5. Start container
    cmd := exec.Command("podman", args...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("failed to start container: %w\nOutput: %s", err, string(output))
    }
    
    containerID := strings.TrimSpace(string(output))
    
    container := &Container{
        Name:    cfg.Name,
        Image:   cfg.Image,
        ID:      containerID,
        Network: cfg.Network,
        Ports:   cfg.Ports,
    }
    
    // 6. Health check
    if cfg.HealthCheck != nil {
        fmt.Fprintf(writer, "   ⏳ Waiting for health check...\n")
        if err := performHealthCheck(*cfg.HealthCheck, cfg.Name, writer); err != nil {
            // Show logs on health check failure
            logs := getContainerLogs(cfg.Name)
            return nil, fmt.Errorf("health check failed: %w\nLogs:\n%s", err, logs)
        }
        fmt.Fprintf(writer, "   ✅ Container healthy\n")
    }
    
    fmt.Fprintf(writer, "✅ Container started: %s\n", cfg.Name)
    return container, nil
}

// StopContainer stops and removes a container
func StopContainer(name string, writer io.Writer) error {
    fmt.Fprintf(writer, "🛑 Stopping container: %s\n", name)
    cleanup(name, writer)
    return nil
}

// buildImage builds a container image using podman build
func buildImage(cfg *ContainerBuildConfig, writer io.Writer) error {
    contextDir := cfg.ContextDir
    if contextDir == "" {
        contextDir = getProjectRoot()
    }
    
    args := []string{"build", "-t", cfg.Tag, "-f", cfg.Dockerfile}
    
    for key, value := range cfg.Args {
        args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
    }
    
    args = append(args, contextDir)
    
    cmd := exec.Command("podman", args...)
    cmd.Stdout = writer
    cmd.Stderr = writer
    
    return cmd.Run()
}

// ensureNetwork creates a network if it doesn't exist
func ensureNetwork(name string, writer io.Writer) error {
    // Check if network exists
    cmd := exec.Command("podman", "network", "exists", name)
    if err := cmd.Run(); err == nil {
        // Network exists
        return nil
    }
    
    // Create network
    cmd = exec.Command("podman", "network", "create", name)
    output, err := cmd.CombinedOutput()
    if err != nil {
        // Ignore error if network already exists (race condition)
        if !strings.Contains(string(output), "already exists") {
            return fmt.Errorf("failed to create network: %w\nOutput: %s", err, string(output))
        }
    }
    
    return nil
}

// cleanup stops and removes a container
func cleanup(name string, writer io.Writer) {
    exec.Command("podman", "stop", name).Run() // Ignore errors
    exec.Command("podman", "rm", name).Run()   // Ignore errors
}

// performHealthCheck validates container health
func performHealthCheck(cfg HealthCheckConfig, containerName string, writer io.Writer) error {
    interval := cfg.Interval
    if interval == 0 {
        interval = 1 * time.Second
    }
    
    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 30 * time.Second
    }
    
    start := time.Now()
    
    for {
        var err error
        
        switch {
        case cfg.URL != "":
            // HTTP health check
            expectedStatus := cfg.ExpectedStatus
            if expectedStatus == 0 {
                expectedStatus = 200
            }
            err = httpHealthCheck(cfg.URL, expectedStatus)
            
        case cfg.Port > 0:
            // TCP health check
            err = tcpHealthCheck(cfg.Port)
            
        case len(cfg.ExecCommand) > 0:
            // Exec health check
            err = execHealthCheck(containerName, cfg.ExecCommand)
            
        default:
            return fmt.Errorf("no health check configured")
        }
        
        if err == nil {
            return nil // Healthy!
        }
        
        // Check timeout
        if time.Since(start) > timeout {
            return fmt.Errorf("timeout after %v: %w", timeout, err)
        }
        
        time.Sleep(interval)
    }
}

// httpHealthCheck performs HTTP health check
func httpHealthCheck(url string, expectedStatus int) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != expectedStatus {
        return fmt.Errorf("expected status %d, got %d", expectedStatus, resp.StatusCode)
    }
    
    return nil
}

// tcpHealthCheck performs TCP health check
func tcpHealthCheck(port int) error {
    conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 5*time.Second)
    if err != nil {
        return err
    }
    conn.Close()
    return nil
}

// execHealthCheck performs exec command health check
func execHealthCheck(containerName string, command []string) error {
    args := append([]string{"exec", containerName}, command...)
    cmd := exec.Command("podman", args...)
    return cmd.Run()
}

// getContainerLogs retrieves container logs for debugging
func getContainerLogs(name string) string {
    cmd := exec.Command("podman", "logs", "--tail", "50", name)
    output, _ := cmd.CombinedOutput()
    return string(output)
}
```

---

## 🎯 **Part 3: Usage Examples**

### **Example 1: AIAnalysis with HolmesGPT API**

```go
// test/infrastructure/aianalysis.go

func StartAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    
    // 1. Start DataStorage stack (opinionated)
    dsConfig := DSBootstrapConfig{
        ServiceName:      "aianalysis",
        PostgresPort:     15438,
        RedisPort:        16384,
        DataStoragePort:  18095,
        MetricsPort:      19095,
        MigrationsDir:    "migrations",
        ConfigDir:        filepath.Join(projectRoot, "test/integration/aianalysis/config"),
        PostgresUser:     "slm_user",
        PostgresPassword: "test_password",
        PostgresDB:       "action_history",
    }
    
    dsInfra, err := StartDSBootstrap(dsConfig, writer)
    if err != nil {
        return fmt.Errorf("failed to start DataStorage: %w", err)
    }
    
    // 2. Start HolmesGPT API (generic container)
    hapiConfig := ContainerConfig{
        Name:  "aianalysis_holmesgpt_1",
        Image: "localhost/kubernaut-holmesgpt-api:latest",
        BuildContext: &ContainerBuildConfig{
            Dockerfile: "holmesgpt-api/Dockerfile",
            ContextDir: projectRoot,
            Tag:        "localhost/kubernaut-holmesgpt-api:latest",
        },
        Ports: map[int]int{
            18120: 8080, // HolmesGPT API
        },
        Env: map[string]string{
            "MOCK_LLM_MODE":     "true",
            "DATASTORAGE_URL":   fmt.Sprintf("http://%s:8080", dsInfra.DataStorageContainer),
        },
        Network: "aianalysis_test-network",
        HealthCheck: &HealthCheckConfig{
            URL:     "http://localhost:18120/health",
            Timeout: 60 * time.Second,
        },
    }
    
    hapiContainer, err := StartContainer(hapiConfig, writer)
    if err != nil {
        StopDSBootstrap(dsInfra, writer)
        return fmt.Errorf("failed to start HolmesGPT API: %w", err)
    }
    
    // Store infrastructure references for cleanup
    return nil
}
```

### **Example 2: Gateway (Simpler - just DS)**

```go
// test/infrastructure/gateway.go

func StartGatewayIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    
    // Gateway just needs DataStorage (opinionated bootstrap)
    dsConfig := DSBootstrapConfig{
        ServiceName:      "gateway",
        PostgresPort:     15437,
        RedisPort:        16383,
        DataStoragePort:  18091,
        MetricsPort:      19091,
        MigrationsDir:    "migrations",
        ConfigDir:        filepath.Join(projectRoot, "test/integration/gateway/config"),
        PostgresUser:     "slm_user",
        PostgresPassword: "test_password",
        PostgresDB:       "action_history",
    }
    
    _, err := StartDSBootstrap(dsConfig, writer)
    return err
}
```

### **Example 3: WorkflowExecution with Additional Service**

```go
// test/infrastructure/workflowexecution.go

func StartWEIntegrationInfrastructure(writer io.Writer) error {
    // 1. DataStorage bootstrap
    dsConfig := DSBootstrapConfig{
        ServiceName:     "workflowexecution",
        PostgresPort:    15441,
        RedisPort:       16387,
        DataStoragePort: 18097,
        MetricsPort:     19097,
        // ... other config ...
    }
    
    dsInfra, err := StartDSBootstrap(dsConfig, writer)
    if err != nil {
        return err
    }
    
    // 2. Optional: Start Tekton mock or other service (generic container)
    // ... use StartContainer() for additional services
    
    return nil
}
```

---

## 📊 **Benefits**

### **Opinionated DS Bootstrap**
- ✅ **95% Code Reuse**: Same PostgreSQL + Redis + DataStorage logic for all services
- ✅ **DD-TEST-001 Compliant**: Port configuration per service
- ✅ **DD-TEST-002 Compliant**: Sequential startup with health checks
- ✅ **Consistent Behavior**: All services use same pattern
- ✅ **Easy Migration**: Gateway, RO, NT, SP, WE can all migrate

### **Generic Container Orchestration**
- ✅ **Flexible**: Any service (HolmesGPT API, Embedding, Tekton Mock, etc.)
- ✅ **Declarative**: Configuration-driven, not imperative code
- ✅ **Testable**: Each component can be tested independently
- ✅ **Debuggable**: Automatic log capture on health check failure
- ✅ **Network-Aware**: Automatic network creation and management

---

## 📋 **Migration Strategy**

### **Phase 1: Extend `datastorage_bootstrap.go`** (30 minutes)
- Add optional fields (PostgresImage, RedisImage, DataStorageEnv, SkipRedis)
- Validate with Gateway (already using shared bootstrap pattern)

### **Phase 2: Create `container.go`** (2 hours)
- Implement ContainerConfig and StartContainer()
- Add health check abstractions (HTTP, TCP, exec)
- Add tests for container orchestration

### **Phase 3: Migrate AIAnalysis** (1 hour)
- Use `StartDSBootstrap()` for PostgreSQL + Redis + DataStorage
- Use `StartContainer()` for HolmesGPT API
- Remove `podman-compose.yml` dependency
- Update ports to DD-TEST-001 v1.6

### **Phase 4: Migrate Other Services** (1 hour each)
- RemediationOrchestrator: Use shared bootstrap
- Notification: Use shared bootstrap
- WorkflowExecution: Use shared bootstrap
- SignalProcessing: Use shared bootstrap

---

## ✅ **Success Criteria**

- ✅ All services use `StartDSBootstrap()` for DataStorage stack
- ✅ AIAnalysis uses `StartContainer()` for HolmesGPT API
- ✅ Zero `podman-compose.yml` files for multi-service dependencies
- ✅ All services DD-TEST-001 + DD-TEST-002 compliant
- ✅ Code duplication < 5% across integration test infrastructure

---

## 🎯 **Recommendation**

**APPROVE** this proposal with implementation priority:

1. **Immediate**: Extend `datastorage_bootstrap.go` (already exists, minimal work)
2. **Short-term**: Create `container.go` (2 hours, high reuse value)
3. **Short-term**: Migrate AIAnalysis (unblocks parallel testing, fixes DD-TEST-002 violation)
4. **Medium-term**: Migrate remaining services (consistent pattern across codebase)

**Estimated Total Effort**: 6-8 hours for complete migration of all services

---

**Document Status**: 💡 **PROPOSAL** - Awaiting User Approval  
**Confidence**: **95%** (proven pattern from Gateway + existing datastorage_bootstrap.go)  
**Recommended Action**: Approve and proceed with Phase 1 (extend datastorage_bootstrap.go)


