# Healthcheck Domain Reference Architecture

## Executive Summary

The Healthcheck domain (`reference/internal/healthcheck/`) provides health checking capabilities for DNS services and other components. It implements a flexible checker interface with aggregation support, allowing multiple health checks to be combined for overall system health assessment.

## Architecture Overview

### Current Architecture Pattern

The Healthcheck domain follows a **checker interface pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Healthcheck Domain                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Checker      │  │ DNS Checker  │  │ Handler     │      │
│  │ Interface    │  │              │  │             │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Interface-based design for extensibility
- Component-specific checkers
- Health check aggregation
- Latency measurement
- Retry and timeout support

## Core Components

### 1. Checker Interface

**Location:** `internal/healthcheck/healthcheck.go`

**Responsibilities:**
- Common interface for all health checkers
- Health probe execution
- Latency measurement
- Healthy state waiting

**Key Operations:**
```go
type Checker interface {
    Name() string
    CheckOnce() (ok bool, latency time.Duration, err error)
    WaitHealthy() bool
}
```

**Interface Methods:**
- `Name()` - Component name for identification
- `CheckOnce()` - Single health probe with latency
- `WaitHealthy()` - Block until healthy or timeout

### 2. Result Structure

**Location:** `internal/healthcheck/healthcheck.go`

**Responsibilities:**
- Health check result representation
- Error information
- Latency tracking

**Structure:**
```go
type Result struct {
    Healthy bool
    Latency time.Duration
    Error   error
}
```

### 3. Aggregation

**Location:** `internal/healthcheck/healthcheck.go` (Aggregate)

**Responsibilities:**
- Run multiple checkers in parallel
- Aggregate results
- Overall health determination

**Key Operations:**
```go
Aggregate(checkers ...Checker) (map[string]Result, bool)
```

**Aggregation Logic:**
- Run all checkers concurrently
- Collect individual results
- Determine overall health (all must be healthy)
- Return per-component results

### 4. DNS Checker

**Location:** `internal/healthcheck/dns.go`

**Responsibilities:**
- DNS service health checking
- DNS query execution
- Retry logic
- Timeout handling

**Key Operations:**
```go
NewDNSHealthChecker(server string, timeout time.Duration, maxRetries int, retryDelay time.Duration) Checker
```

**Health Check Logic:**
- Execute DNS query to health check endpoint
- Retry on failure with configurable delay
- Measure latency
- Return health status

### 5. Health Handler

**Location:** `internal/healthcheck/handler.go`

**Responsibilities:**
- HTTP endpoint for health checks
- Component status reporting
- JSON response formatting

**Key Operations:**
```go
NewHandler(dnsChecker, syncManager, proxyManager) (*Handler, error)
ServeHTTP(w http.ResponseWriter, r *http.Request)
```

**Response Format:**
```json
{
  "status": "healthy",
  "components": {
    "dns": {
      "healthy": true,
      "latency": "10ms"
    },
    "sync": {
      "healthy": true,
      "latency": "5ms"
    }
  }
}
```

## Data Flow

### Current Flow: Health Check Request

```
1. HTTP GET /health
   ↓
2. HealthHandler.ServeHTTP()
   ↓
3. Aggregate all checkers
   ├─→ DNS Checker
   ├─→ Sync Manager Checker
   └─→ Proxy Manager Checker
   ↓
4. Determine overall status
   ↓
5. Return JSON response
```

### Current Flow: DNS Health Check

```
1. CheckOnce()
   ↓
2. Execute DNS query
   ├─→ Query health check endpoint
   └─→ Measure latency
   ↓
3. Check result
   ├─→ Success → return healthy
   └─→ Failure → retry or return unhealthy
```

## CRD Mapping Considerations

### Kubernetes-Native Health Checks

**Option 1: Kubernetes Probes**
- Use liveness and readiness probes
- Built-in health check support
- Automatic pod restart on failure

**Option 2: Health Check Endpoint**
- Maintain HTTP health check endpoint
- Use for Kubernetes probes
- Component status reporting

**Option 3: Status Subresource**
- Use CRD status for health information
- Controller reports health in status
- Kubernetes-native status management

**Recommended Approach:**
- **Option 1 + Option 3** - Combine Kubernetes probes with CRD status
- Use liveness/readiness probes for pod health
- Use CRD status for resource health
- Optional: Maintain HTTP endpoint for detailed status

## Key Migration Considerations

### 1. Kubernetes Probes

**Current:** Custom health check interface and handlers
**Target:** Kubernetes liveness and readiness probes

**Migration:**
- Implement HTTP health check endpoint
- Configure liveness probe in Deployment
- Configure readiness probe in Deployment
- Use probe for automatic pod management

### 2. Component Health

**Current:** Aggregated component health checks
**Target:** CRD status conditions

**Migration:**
- Report component health in CRD status
- Use status conditions for health information
- Controller updates status based on component health
- Remove custom aggregation logic

### 3. Health Check Interface

**Current:** Checker interface for extensibility
**Target:** Controller reconciliation for health

**Migration:**
- Replace checker interface with controller logic
- Health checks in reconciliation loop
- Status updates based on health checks
- Maintain interface pattern if needed for internal checks

### 4. Latency Measurement

**Current:** Latency tracking in health checks
**Target:** Metrics and observability

**Migration:**
- Use Prometheus metrics for latency
- Remove latency from health check responses
- Use metrics for performance monitoring
- Optional: Include latency in status if needed

### 5. Retry Logic

**Current:** Retry logic in health checkers
**Target:** Kubernetes probe retry configuration

**Migration:**
- Configure retry in probe settings
- Use probe failureThreshold
- Remove custom retry logic
- Leverage Kubernetes retry mechanisms

## Kubernetes Probe Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dns-operator
spec:
  template:
    spec:
      containers:
      - name: operator
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
```

## Testing Strategy

### Current Testing Approach
- Unit tests for checkers
- Aggregation tests
- Mock DNS queries

### Target Testing Approach
- Probe configuration tests
- Status condition tests
- Integration tests with testenv
- Metrics tests

## Summary

The Healthcheck domain provides health checking capabilities for system components. Migration to Kubernetes will:

1. **Kubernetes Probes** - Use liveness and readiness probes for pod health
2. **CRD Status** - Use status conditions for resource health
3. **Metrics** - Use Prometheus metrics for latency and performance
4. **Controller Health** - Health checks in reconciliation loop
5. **Status Reporting** - Component health in CRD status

The domain's checker interface and aggregation logic can be adapted for controller-based health checking, while leveraging Kubernetes-native probe mechanisms for pod health management.


