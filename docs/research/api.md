# API Domain Reference Architecture

## Executive Summary

The API domain (`reference/internal/api/`) provides the HTTP layer for the DNS Manager service. It implements a RESTful API with route registration, request handling, and response management. The domain follows a modular architecture with centralized route registry and handler management.

## Architecture Overview

### Current Architecture Pattern

The API domain follows a **modular handler registry pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    HandlerRegistry                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ RecordHandler│  │ HealthHandler│  │ DeviceHandler│      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │ DocsHandler  │  │ RouteRegistry│                        │
│  └──────────────┘  └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Centralized handler registry (`HandlerRegistry`)
- Route registration via `RouteInfo` structs
- Modular handler initialization
- Dependency injection for handlers
- HTTP ServeMux for routing

## Core Components

### 1. Handler Registry

**Location:** `internal/api/handler/handler.go`, `internal/api/handler/registry.go`

**Responsibilities:**
- Centralized management of all HTTP handlers
- Handler initialization with dependencies
- Route registration and HTTP ServeMux management
- Handler lifecycle coordination

**Key Operations:**
```go
NewHandlerRegistry(dnsManager, dnsChecker, syncManager, proxyManager, tailscaleClient, certificateManager) (*HandlerRegistry, error)
RegisterHandlers(mux *http.ServeMux)
GetServeMux() *http.ServeMux
```

**Dependencies:**
- DNS Manager (CoreDNS)
- Health Checker
- Sync Manager
- Proxy Manager
- Tailscale Client
- Certificate Manager

### 2. Record Handler

**Location:** `internal/api/handler/record.go`

**Responsibilities:**
- DNS record creation via `/add-record` endpoint
- DNS record listing via `/list-records` endpoint
- DNS record removal via `/remove-record` endpoint
- Request validation and normalization
- Integration with certificate manager for SAN management

**Key Operations:**
```go
AddRecord(w http.ResponseWriter, r *http.Request)
ListRecords(w http.ResponseWriter, r *http.Request)
RemoveRecord(w http.ResponseWriter, r *http.Request)
```

**Request/Response Flow:**
1. HTTP request received
2. Request validation and normalization
3. Record service orchestration (DNS + Proxy)
4. Certificate SAN management (if applicable)
5. HTTP response with status code

### 3. Route Registry

**Location:** `internal/api/handler/registry.go`, `internal/api/types/route.go`

**Responsibilities:**
- Centralized route registration
- Route metadata management (`RouteInfo`)
- Route discovery for OpenAPI generation
- Module-based route organization

**RouteInfo Structure:**
```go
type RouteInfo struct {
    Path        string
    Method      string
    Handler     http.HandlerFunc
    Module      string
    Description string
    RequestType interface{}
    ResponseType interface{}
}
```

**Key Operations:**
```go
RegisterRoute(route RouteInfo)
GetRegisteredRoutes() []RouteInfo
```

### 4. API Types

**Location:** `internal/api/types/route.go`

**Responsibilities:**
- Route metadata structures
- Type definitions for API layer
- Request/response type definitions

## Data Flow

### Current Flow: HTTP Request → DNS Record Creation

```
1. HTTP POST /add-record
   ↓
2. HandlerRegistry routes to RecordHandler
   ↓
3. RecordHandler.AddRecord()
   ├─→ Request validation
   ├─→ RecordService.CreateRecord()
   │   ├─→ DNSManager.AddRecord()
   │   ├─→ ProxyManager.AddRule()
   │   └─→ CertificateManager.AddDomainToSAN()
   └─→ HTTP 201 Created response
```

## CRD Mapping Considerations

### Target Architecture: Kubernetes API Server

**Option 1: Native Kubernetes API**
- Use Kubernetes API server directly
- CRDs managed via `kubectl` or Kubernetes client libraries
- No HTTP API layer needed

**Option 2: API Server with CRD Backend**
- Maintain REST API for convenience
- API server creates/updates CRDs
- Kubernetes API server as backend
- Provides backward compatibility

**Option 3: Webhook-based API**
- Validating/Mutating webhooks for CRD operations
- Custom API server for additional endpoints
- Kubernetes-native validation

**Recommended Approach:**
- **Option 1** for primary interface (Kubernetes-native)
- **Option 2** as optional convenience layer if needed
- Webhooks for validation and mutation

## Key Migration Considerations

### 1. Handler to Controller Mapping

**Current:** HTTP handlers process requests synchronously
**Target:** Controllers watch CRDs and reconcile state

**Mapping:**
- `RecordHandler.AddRecord()` → `DNSRecordController` reconciliation
- `RecordHandler.ListRecords()` → `kubectl get dnsrecords`
- `RecordHandler.RemoveRecord()` → `kubectl delete dnsrecord`

### 2. Route Registration

**Current:** Route registration via `RouteInfo` structs
**Target:** Kubernetes API endpoints or webhook endpoints

**Migration:**
- Remove HTTP route registration
- Use Kubernetes API server endpoints
- Optional: Maintain route registry for webhook endpoints

### 3. Request Validation

**Current:** Validation in handlers
**Target:** CRD validation via OpenAPI schema or webhooks

**Migration:**
- Move validation to CRD OpenAPI schema
- Use validating webhooks for complex validation
- Remove handler-level validation

### 4. Response Format

**Current:** JSON HTTP responses
**Target:** Kubernetes API responses or CRD status

**Migration:**
- Replace HTTP responses with CRD status updates
- Use Kubernetes API response format
- Maintain JSON format for webhook responses

## Testing Strategy

### Current Testing Approach
- HTTP handler unit tests
- Request/response mocking
- Integration tests with test HTTP server

### Target Testing Approach
- Controller unit tests with fake client
- CRD validation tests
- Webhook validation tests
- Integration tests with testenv

## Summary

The API domain provides the HTTP interface for DNS management operations. Migration to Kubernetes will:

1. **Replace HTTP handlers with controllers** - Controllers watch CRDs instead of handling HTTP requests
2. **Use Kubernetes API server** - CRDs managed via Kubernetes API instead of custom HTTP endpoints
3. **Optional API server** - Maintain REST API as convenience layer if needed
4. **Webhook validation** - Use validating/mutating webhooks for request validation

The route registry and handler patterns can be adapted for webhook endpoints if maintaining an HTTP API layer is desired.


