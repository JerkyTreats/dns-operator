# OpenAPI Generation Command Reference Architecture

## Executive Summary

The OpenAPI Generation command from the legacy reference repo under `cmd/generate-openapi/` provides build-time OpenAPI specification generation by analyzing Go code using AST based analysis. It discovers routes registered via `RouteInfo` structs and generates complete OpenAPI 3.0 specifications automatically.

## Architecture Overview

### Current Architecture Pattern

The OpenAPI Generation command follows an **AST-based code analysis pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    OpenAPI Generator                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ AST Analyzer │  │ Type Schema  │  │ Spec Builder │      │
│  │              │  │ Generator    │  │              │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- AST-based route discovery
- Type introspection for schema generation
- OpenAPI 3.0 specification generation
- Zero-maintenance documentation
- Build-time generation

## Core Components

### 1. Generator

**Location:** `cmd/generate-openapi/analyzer/generator.go`

**Responsibilities:**
- Route discovery via AST analysis
- RouteInfo extraction
- Type schema generation
- OpenAPI spec building

**Key Operations:**
```go
NewGenerator() *Generator
GenerateSpec() (string, error)
GetDiscoveredRoutes() []RouteInfo
```

**Generation Flow:**
1. Analyze Go code using AST
2. Discover RouteInfo registrations
3. Extract route metadata
4. Generate type schemas
5. Build OpenAPI specification

### 2. AST Analyzer

**Location:** `cmd/generate-openapi/analyzer/generator.go`

**Responsibilities:**
- Go code parsing
- AST traversal
- RouteInfo discovery
- Function call analysis

**Analysis Process:**
- Parse Go source files
- Traverse AST nodes
- Find RegisterRoute() calls
- Extract RouteInfo structs
- Collect route metadata

### 3. Type Schema Generator

**Location:** `cmd/generate-openapi/analyzer/spec.go`

**Responsibilities:**
- Type introspection
- JSON schema generation
- Request/response schema creation
- Nested type handling

**Key Operations:**
```go
GenerateSchema(typ reflect.Type) map[string]interface{}
```

**Schema Generation:**
- Reflect on Go types
- Generate JSON schemas
- Handle nested types
- Support slices and maps
- Extract validation tags

### 4. Spec Builder

**Location:** `cmd/generate-openapi/analyzer/spec.go`

**Responsibilities:**
- OpenAPI 3.0 specification construction
- Route grouping
- Path and operation definitions
- Response schema integration

**Key Operations:**
```go
BuildSpec(routes []RouteInfo) (string, error)
```

**Spec Structure:**
- OpenAPI 3.0 format
- Paths and operations
- Request/response schemas
- Route descriptions
- Module grouping

### 5. Main Command

**Location:** `cmd/generate-openapi/main.go`

**Responsibilities:**
- Command-line interface
- Output file management
- Package import for route registration
- Generation orchestration

**Key Operations:**
```go
main()
```

**Command Flags:**
- `-output`: Output file path (default: `docs/api/openapi.yaml`)
- `-verbose`: Enable verbose logging

## Data Flow

### Current Flow: OpenAPI Generation

```
1. Import packages to trigger route registration
   ↓
2. Create generator
   ↓
3. GenerateSpec()
   ├─→ Analyze Go code with AST
   ├─→ Discover RouteInfo registrations
   ├─→ Extract route metadata
   ├─→ Generate type schemas
   └─→ Build OpenAPI specification
   ↓
4. Write to output file
   ↓
5. Generation complete
```

### Current Flow: Route Discovery

```
1. Parse Go source files
   ↓
2. Traverse AST
   ↓
3. Find RegisterRoute() calls
   ↓
4. Extract RouteInfo struct
   ├─→ Path
   ├─→ Method
   ├─→ Module
   ├─→ Description
   ├─→ RequestType
   └─→ ResponseType
   ↓
5. Collect all routes
```

## CRD Mapping Considerations

### Kubernetes CRD OpenAPI Schema

**Option 1: CRD Schema Generation**
- Generate OpenAPI schema for CRDs
- Use kubebuilder markers
- Automatic schema generation
- Kubernetes-native validation

**Option 2: Custom Schema Generation**
- Use AST analysis for CRD schemas
- Generate from Go structs
- Custom validation rules
- Type introspection

**Option 3: Hybrid Approach**
- Use kubebuilder for basic schema
- Custom generation for complex types
- Combine both approaches

**Recommended Approach:**
- **Option 1** - Use kubebuilder markers for CRD schemas
- Kubernetes-native schema generation
- Automatic validation
- Standard CRD tooling

### Webhook OpenAPI Schema

**Option 1: Webhook Schema Generation**
- Generate OpenAPI schema for webhooks
- Use AST analysis for webhook types
- Maintain generation tool
- Custom schema generation

**Option 2: Manual Schema**
- Manually define webhook schemas
- Use OpenAPI spec directly
- Less automation
- More control

**Recommended Approach:**
- **Option 1** - Adapt generation tool for webhooks
- Reuse AST analysis logic
- Generate webhook request/response schemas
- Maintain zero-maintenance approach

## Key Migration Considerations

### 1. Route Discovery

**Current:** AST analysis of RouteInfo registrations
**Target:** CRD schema generation from Go structs

**Migration:**
- Adapt AST analysis for CRD structs
- Generate OpenAPI schema from CRD types
- Use kubebuilder markers
- Maintain type introspection

### 2. Schema Generation

**Current:** Request/response schema generation
**Target:** CRD spec/status schema generation

**Migration:**
- Generate CRD OpenAPI schema
- Use kubebuilder for basic generation
- Custom generation for complex types
- Maintain type introspection logic

### 3. Webhook Schema

**Current:** REST API schema generation
**Target:** Webhook request/response schema generation

**Migration:**
- Adapt generator for webhook types
- Generate webhook OpenAPI schemas
- Maintain AST analysis approach
- Support webhook validation schemas

### 4. Build Integration

**Current:** Standalone generation command
**Target:** Integrated with kubebuilder

**Migration:**
- Use kubebuilder for CRD schemas
- Maintain custom generator for webhooks
- Integrate with build process
- CI/CD integration

### 5. Documentation

**Current:** OpenAPI spec for REST API
**Target:** CRD documentation and webhook docs

**Migration:**
- Generate CRD documentation
- Generate webhook documentation
- Maintain OpenAPI spec format
- Use for API documentation

## Kubebuilder Integration Example

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FQDN",type=string,JSONPath=.status.fqdn
type DNSRecord struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    
    Spec   DNSRecordSpec   `json:"spec,omitempty"`
    Status DNSRecordStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`
type DNSRecordSpec struct {
    FQDN string `json:"fqdn"`
    // ...
}
```

## Testing Strategy

### Current Testing Approach
- Generator unit tests
- AST analysis tests
- Schema generation tests
- Spec building tests

### Target Testing Approach
- Continue generator tests
- CRD schema generation tests
- Webhook schema tests
- Integration tests with kubebuilder

## Summary

The OpenAPI Generation command provides build-time OpenAPI specification generation via AST analysis. Migration to Kubernetes will:

1. **CRD Schema Generation** - Use kubebuilder for CRD OpenAPI schemas
2. **Webhook Schema Generation** - Adapt generator for webhook schemas
3. **Type Introspection** - Maintain AST analysis for complex types
4. **Build Integration** - Integrate with kubebuilder build process
5. **Documentation** - Generate CRD and webhook documentation

The domain's AST analysis and type introspection logic should be preserved and adapted for CRD and webhook schema generation, while leveraging kubebuilder for standard CRD schema generation.

