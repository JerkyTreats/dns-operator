# Documentation Domain Reference Architecture

## Executive Summary

The Documentation domain (`reference/internal/docs/`) provides HTTP handlers for serving Swagger UI and OpenAPI specifications. It enables interactive API documentation with automatic protocol detection (HTTP/HTTPS) and theme support.

## Architecture Overview

### Current Architecture Pattern

The Documentation domain follows a **handler-based serving pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    DocsHandler                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Swagger UI   │  │ OpenAPI Spec │  │ Static Files │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Swagger UI integration
- OpenAPI specification serving
- Protocol detection (HTTP/HTTPS)
- Theme support (dark/light)
- Static file serving

## Core Components

### 1. Docs Handler

**Location:** `internal/docs/handler.go`

**Responsibilities:**
- Swagger UI HTML generation
- OpenAPI specification file serving
- Static documentation file serving
- Protocol detection and URL generation

**Key Operations:**
```go
NewDocsHandler() (*DocsHandler, error)
ServeSwaggerUI(w http.ResponseWriter, r *http.Request)
ServeOpenAPISpec(w http.ResponseWriter, r *http.Request)
ServeDocs(w http.ResponseWriter, r *http.Request)
```

**Configuration:**
```go
type SwaggerConfig struct {
    Enabled  bool
    Path     string      // /swagger
    SpecPath string      // /docs/openapi.yaml
    UITitle  string
    Theme    string      // dark/light
}
```

### 2. Swagger UI Generation

**Location:** `internal/docs/handler.go` (generateSwaggerHTML)

**Responsibilities:**
- HTML page generation with Swagger UI
- Protocol detection (HTTP vs HTTPS)
- Base URL construction
- Theme CSS injection

**Key Features:**
- Automatic protocol detection from request
- Support for proxy headers (X-Forwarded-Proto)
- Fallback host detection
- Dark/light theme support

### 3. OpenAPI Spec Serving

**Location:** `internal/docs/handler.go` (ServeOpenAPISpec)

**Responsibilities:**
- Read OpenAPI specification file
- Serve YAML content
- CORS headers for Swagger UI
- Error handling for missing files

**File Location:**
- Default: `docs/api/openapi.yaml`
- Configurable via SwaggerConfig

## Data Flow

### Current Flow: Swagger UI Request

```
1. HTTP GET /swagger
   ↓
2. DocsHandler.ServeSwaggerUI()
   ↓
3. Detect protocol (HTTP/HTTPS)
   ├─→ Check TLS connection
   ├─→ Check X-Forwarded-Proto header
   └─→ Construct base URL
   ↓
4. Generate Swagger UI HTML
   ├─→ Include Swagger UI assets (CDN)
   ├─→ Set OpenAPI spec URL
   └─→ Apply theme CSS
   ↓
5. Return HTML response
```

### Current Flow: OpenAPI Spec Request

```
1. HTTP GET /docs/openapi.yaml
   ↓
2. DocsHandler.ServeOpenAPISpec()
   ↓
3. Read OpenAPI spec file
   ↓
4. Set CORS headers
   ↓
5. Return YAML content
```

## CRD Mapping Considerations

### Kubernetes-Native Documentation

**Option 1: Remove HTTP Documentation**
- Documentation not needed in Kubernetes operator
- Use `kubectl explain` for CRD documentation
- Use Kubernetes API documentation tools

**Option 2: Webhook Documentation**
- Serve documentation for webhook endpoints
- Maintain Swagger UI for webhook API
- OpenAPI spec for webhook validation

**Option 3: Operator Metrics/Docs Endpoint**
- Optional metrics and documentation endpoint
- Serve operator-specific documentation
- Health check and status information

**Recommended Approach:**
- **Option 1** - Remove HTTP documentation layer
- Use Kubernetes-native documentation tools
- CRD documentation via OpenAPI schema
- Optional: Operator status/metrics endpoint

## Key Migration Considerations

### 1. Swagger UI Removal

**Current:** Swagger UI for REST API documentation
**Target:** Kubernetes CRD documentation

**Migration:**
- Remove Swagger UI handler
- Use `kubectl explain` for CRD documentation
- Use Kubernetes API documentation
- Optional: Operator-specific docs endpoint

### 2. OpenAPI Specification

**Current:** OpenAPI spec for REST API
**Target:** OpenAPI schema for CRDs

**Migration:**
- Generate OpenAPI schema for CRDs
- Use CRD OpenAPI schema for validation
- Remove REST API OpenAPI spec
- Maintain OpenAPI schema for webhooks if needed

### 3. Protocol Detection

**Current:** HTTP/HTTPS protocol detection for Swagger UI
**Target:** Not needed for Kubernetes

**Migration:**
- Remove protocol detection logic
- Kubernetes handles TLS termination
- Use Kubernetes service URLs

### 4. Static File Serving

**Current:** Static documentation file serving
**Target:** Not needed for Kubernetes

**Migration:**
- Remove static file serving
- Use Kubernetes-native documentation
- Optional: ConfigMap-based documentation

## Testing Strategy

### Current Testing Approach
- Handler unit tests
- Swagger UI generation tests
- OpenAPI spec serving tests

### Target Testing Approach
- CRD OpenAPI schema validation tests
- Webhook documentation tests (if applicable)
- Operator status endpoint tests (if applicable)

## Summary

The Documentation domain provides Swagger UI and OpenAPI specification serving for the REST API. Migration to Kubernetes will:

1. **Remove HTTP Documentation** - Not needed for Kubernetes operator
2. **CRD OpenAPI Schema** - Use Kubernetes CRD OpenAPI schema for documentation
3. **Kubernetes Native Docs** - Use `kubectl explain` and Kubernetes API documentation
4. **Optional Status Endpoint** - Operator-specific status/metrics endpoint if needed

The domain's functionality is largely replaced by Kubernetes-native documentation tools, though the OpenAPI generation logic may be useful for CRD schema generation.


