# API Server Command Reference Architecture

## Executive Summary

The API Server command (`reference/cmd/api/`) is the main entry point for the DNS Manager service. It initializes all components, sets up HTTP/HTTPS servers, manages service lifecycle, and coordinates background processes including certificate management, device synchronization, and firewall setup.

## Architecture Overview

### Current Architecture Pattern

The API Server command follows a **monolithic service initialization pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    API Server (main.go)                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Config Init  │  │ Component    │  │ HTTP Server  │      │
│  │              │  │ Init         │  │              │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │ Background   │  │ Lifecycle    │                        │
│  │ Processes    │  │ Management   │                        │
│  └──────────────┘  └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Single binary entry point
- Component initialization and coordination
- HTTP/HTTPS server management
- Background process management
- Graceful shutdown handling

## Core Components

### 1. Configuration Initialization

**Location:** `cmd/api/main.go` (main function)

**Responsibilities:**
- Configuration file loading
- Required key validation
- Environment variable support
- Configuration validation

**Key Operations:**
```go
config.FirstTimeInit(configFile)
config.CheckRequiredKeys()
```

**Configuration Keys:**
- Server configuration (host, port, TLS)
- DNS configuration
- Tailscale configuration
- Certificate configuration
- Logging configuration

### 2. Component Initialization

**Location:** `cmd/api/main.go` (main function)

**Responsibilities:**
- Tailscale client initialization
- DNS manager setup
- Proxy manager initialization
- Certificate manager setup
- Firewall manager setup
- Handler registry initialization

**Initialization Order:**
1. Configuration
2. Tailscale client
3. Firewall manager
4. DNS manager
5. Proxy manager
6. Health checker
7. Certificate manager (if enabled)
8. Sync manager (if enabled)
9. Handler registry

### 3. HTTP Server

**Location:** `cmd/api/main.go` (main function)

**Responsibilities:**
- HTTP server setup and configuration
- HTTPS server setup (if enabled)
- Handler registration
- Server lifecycle management

**Key Operations:**
```go
server := &http.Server{
    Addr:         fmt.Sprintf("%s:%d", host, port),
    ReadTimeout:  readTimeout,
    WriteTimeout: writeTimeout,
    IdleTimeout:  idleTimeout,
    Handler:      mux,
}
```

**Server Configuration:**
- Host and port from config
- Timeout configuration
- Handler registration via HandlerRegistry
- Graceful shutdown handling

### 4. HTTPS Server

**Location:** `cmd/api/main.go` (main function)

**Responsibilities:**
- TLS server setup
- Certificate loading
- Dual HTTP/HTTPS support
- Certificate readiness coordination

**Key Operations:**
```go
httpsSrv := &http.Server{
    Addr:      fmt.Sprintf("%s:%d", host, tlsPort),
    Handler:   mux,
    TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
}
httpsSrv.ListenAndServeTLS(certFile, keyFile)
```

**TLS Configuration:**
- Certificate file path
- Key file path
- TLS version (minimum TLS 1.2)
- Certificate readiness channel

### 5. Background Processes

**Location:** `cmd/api/main.go` (main function)

**Responsibilities:**
- Certificate process management
- Device synchronization
- DNS API self-registration
- Background goroutine coordination

**Background Processes:**
- Certificate manager process (if enabled)
- Sync manager polling (if enabled)
- DNS API service registration
- Certificate readiness waiting

### 6. Lifecycle Management

**Location:** `cmd/api/main.go` (main function)

**Responsibilities:**
- Graceful shutdown handling
- Signal handling (SIGINT, SIGTERM)
- Server shutdown coordination
- Resource cleanup

**Key Operations:**
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit
server.Shutdown(ctx)
```

## Data Flow

### Current Flow: Application Startup

```
1. Parse command-line flags
   ↓
2. Initialize configuration
   ├─→ Load config file
   ├─→ Apply environment variables
   └─→ Validate required keys
   ↓
3. Initialize Tailscale client
   ├─→ Get device IP
   └─→ Validate connection
   ↓
4. Initialize firewall manager
   ├─→ Setup firewall rules
   └─→ Validate setup
   ↓
5. Initialize DNS manager
   ├─→ Add base domain
   └─→ Setup zone files
   ↓
6. Initialize proxy manager
   ↓
7. Initialize health checker
   ├─→ Wait for DNS health
   └─→ Validate connectivity
   ↓
8. Initialize certificate manager (if enabled)
   ├─→ Start certificate process
   └─→ Wait for certificate readiness
   ↓
9. Initialize sync manager (if enabled)
   ├─→ Start polling
   └─→ Sync devices
   ↓
10. Initialize handler registry
    ├─→ Create handlers
    └─→ Register routes
    ↓
11. Start HTTP server
    ↓
12. Start HTTPS server (if enabled)
    ├─→ Wait for certificate readiness
    └─→ Start TLS server
    ↓
13. Register DNS API service
    ↓
14. Wait for shutdown signal
```

### Current Flow: Graceful Shutdown

```
1. Receive shutdown signal (SIGINT/SIGTERM)
   ↓
2. Create shutdown context (10s timeout)
   ↓
3. Shutdown HTTP server
   ├─→ Stop accepting new connections
   ├─→ Wait for active connections
   └─→ Close server
   ↓
4. Cleanup resources
   ├─→ Stop background processes
   └─→ Close connections
   ↓
5. Exit
```

## CRD Mapping Considerations

### Kubernetes Operator Pattern

**Option 1: Operator Binary**
- Single operator binary
- Controller-runtime manager
- No HTTP server needed
- Kubernetes-native operation

**Option 2: Operator with Metrics Server**
- Operator binary with metrics endpoint
- Prometheus metrics
- Health check endpoint
- Optional: Webhook server

**Option 3: Hybrid Approach**
- Operator for CRD management
- Optional HTTP API server for convenience
- Separate deployment options

**Recommended Approach:**
- **Option 1** - Pure Kubernetes operator
- Use controller-runtime manager
- No HTTP server for API (use Kubernetes API)
- Optional metrics server
- Optional webhook server

## Key Migration Considerations

### 1. Entry Point Transformation

**Current:** HTTP server with REST API
**Target:** Kubernetes operator with controllers

**Migration:**
- Replace HTTP server with controller-runtime manager
- Remove REST API endpoints
- Use Kubernetes API for resource management
- Optional: Keep metrics/health endpoints

### 2. Component Initialization

**Current:** Manual component initialization
**Target:** Controller initialization via manager

**Migration:**
- Initialize controllers via manager
- Use dependency injection
- Controller-runtime handles lifecycle
- Remove manual initialization

### 3. Background Processes

**Current:** Goroutine-based background processes
**Target:** Controller reconciliation loops

**Migration:**
- Replace background processes with controllers
- Use RequeueAfter for polling
- Use controller reconciliation for periodic tasks
- Remove goroutine management

### 4. Server Lifecycle

**Current:** HTTP/HTTPS server lifecycle
**Target:** Controller manager lifecycle

**Migration:**
- Replace server lifecycle with manager lifecycle
- Use manager.Start() for operation
- Graceful shutdown via context
- Remove server management

### 5. Configuration

**Current:** Config file + environment variables
**Target:** ConfigMap + environment variables

**Migration:**
- Use ConfigMap for operator configuration
- Environment variables for deployment-specific config
- Remove config file loading
- Use controller-runtime config

### 6. Health Checks

**Current:** HTTP health check endpoint
**Target:** Kubernetes probes + CRD status

**Migration:**
- Use liveness/readiness probes
- Report health in CRD status
- Optional: Keep health endpoint for probes
- Remove custom health check logic

## Operator Structure Example

```go
func main() {
    // Setup controller-runtime manager
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme: scheme,
        Metrics: metricsserver.Options{BindAddress: ":8080"},
    })
    
    // Setup controllers
    if err = (&DNSRecordController{}).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "DNSRecord")
        os.Exit(1)
    }
    
    // Start manager
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
```

## Testing Strategy

### Current Testing Approach
- Integration tests with test server
- Component initialization tests
- Lifecycle tests

### Target Testing Approach
- Controller unit tests with fake client
- Manager integration tests
- End-to-end tests with testenv
- Operator deployment tests

## Summary

The API Server command is the main entry point for the DNS Manager service. Migration to Kubernetes will:

1. **Operator Binary** - Replace HTTP server with controller-runtime manager
2. **Controller Initialization** - Initialize controllers via manager
3. **Reconciliation Loops** - Replace background processes with controller reconciliation
4. **Manager Lifecycle** - Use controller-runtime manager lifecycle
5. **ConfigMap Configuration** - Use ConfigMap for operator configuration
6. **Kubernetes Probes** - Use liveness/readiness probes for health

The command's component initialization and coordination logic should be adapted for controller-runtime manager initialization, with background processes replaced by controller reconciliation loops.


