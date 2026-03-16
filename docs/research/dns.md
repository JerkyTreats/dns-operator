# DNS Domain Reference Architecture

## Executive Summary

The DNS domain (`reference/internal/dns/`) manages DNS record creation, zone file management, and CoreDNS integration. It consists of two main sub-packages: `coredns` for CoreDNS server management and `record` for DNS record business logic. The domain handles zone file generation, serial number management, and DNS record validation.

## Architecture Overview

### Current Architecture Pattern

The DNS domain follows a **layered service pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    DNS Domain                                │
│  ┌──────────────────┐  ┌──────────────────┐               │
│  │  Record Service  │  │  CoreDNS Manager │               │
│  │  (Business Logic)│  │  (Zone Files)     │               │
│  └──────────────────┘  └──────────────────┘               │
│  ┌──────────────────┐  ┌──────────────────┐               │
│  │  Generator       │  │  Validator        │               │
│  └──────────────────┘  └──────────────────┘               │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Separation of business logic (record) and infrastructure (coredns)
- Zone file-based DNS management
- Template-based Corefile generation
- Automatic serial number management
- DNS record validation and normalization

## Core Components

### 1. Record Service

**Location:** `internal/dns/record/service.go`

**Responsibilities:**
- DNS record creation orchestration
- Record validation and normalization
- Integration with DNS manager and proxy manager
- Tailscale device IP resolution
- Unified record model management

**Key Operations:**
```go
CreateRecord(req CreateRecordRequest, httpRequest ...*http.Request) (*Record, error)
RemoveRecord(req RemoveRecordRequest) error
ListRecords() ([]Record, error)
```

**Record Model:**
```go
type Record struct {
    Name      string
    Type      string
    Value     string
    TTL       int
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Integration Points:**
- DNS Manager for zone file operations
- Proxy Manager for reverse proxy rules
- Tailscale Client for device IP resolution

### 2. CoreDNS Manager

**Location:** `internal/dns/coredns/manager.go`

**Responsibilities:**
- CoreDNS configuration management
- Zone file creation and updates
- Zone serial number management
- Corefile template generation
- Domain configuration management
- TLS configuration integration

**Key Operations:**
```go
AddRecord(serviceName, name, ip string) error
RemoveRecord(serviceName, name string) error
ListRecords(serviceName string) ([]Record, error)
AddDomain(domain string, tlsConfig *TLSConfig) error
GenerateCorefile() error
```

**Zone File Management:**
- Zone files stored at `/etc/coredns/zones/{domain}.zone`
- Standard DNS zone file format
- SOA, NS, and A record management
- Automatic serial number updates

**Corefile Generation:**
- Template-based Corefile generation
- Domain block configuration
- TLS configuration integration
- Zone file references

### 3. Record Generator

**Location:** `internal/dns/record/generator.go`

**Responsibilities:**
- DNS record generation from zone files
- Record model creation
- Zone file parsing
- Record list generation

**Key Operations:**
```go
GenerateRecordsFromZone(serviceName string) ([]Record, error)
```

**Generation Logic:**
- Parse zone files for A records
- Extract record metadata
- Create Record models
- Handle zone file format variations

### 4. Record Validator

**Location:** `internal/dns/record/validation.go`

**Responsibilities:**
- DNS name validation
- Request normalization
- Input validation
- FQDN validation

**Key Operations:**
```go
ValidateCreateRequest(req CreateRecordRequest) error
NormalizeCreateRequest(req CreateRecordRequest) (CreateRecordRequest, error)
```

**Validation Rules:**
- FQDN format validation
- Service name validation
- Name normalization (lowercase, trim)
- Domain validation

### 5. DNS Name Utilities

**Location:** `internal/dns/coredns/dns_name.go`

**Responsibilities:**
- DNS name manipulation
- FQDN construction
- Name validation
- Domain extraction

**Key Operations:**
```go
BuildFQDN(name, domain string) string
ValidateDNSName(name string) error
```

## Data Flow

### Current Flow: DNS Record Creation

```
1. CreateRecord(request)
   ↓
2. Validate and normalize request
   ↓
3. Resolve DNS Manager IP
   ↓
4. DNSManager.AddRecord()
   ├─→ Load zone file
   ├─→ Add A record
   ├─→ Update serial number
   └─→ Write zone file
   ↓
5. Generate Corefile (if needed)
   ↓
6. Return Record model
```

### Current Flow: Zone File Management

```
1. AddRecord(serviceName, name, ip)
   ↓
2. Load zone file for serviceName
   ↓
3. Add A record entry
   ↓
4. Update SOA serial number
   ↓
5. Write zone file atomically
   ↓
6. Trigger CoreDNS reload (if needed)
```

## CRD Mapping Considerations

### Proposed CRD Structure

```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: webapp
  namespace: default
spec:
  name: webapp
  domain: internal.example.test
  type: A
  targetIP: 100.64.1.5  # Or reference to TailscaleDevice
  ttl: 300
  proxy:
    enabled: true
    targetPort: 8080
    protocol: http
status:
  fqdn: app.internal.example.test
  zoneFile: zone-internal-jerkytreats-dev
  conditions:
    - type: Ready
      status: "True"
```

### Reconciliation Logic

**DNSRecordController Responsibilities:**
- Watch DNSRecord CRDs
- Aggregate DNSRecords by zone (domain)
- Update zone ConfigMaps with all records for that zone
- Manage zone serial numbers
- Handle record conflicts and validation
- Update status with current state

**Zone Management:**
- Each zone has a corresponding ConfigMap (e.g., `zone-{domain}`)
- ConfigMap contains zone file content
- CoreDNS mounts ConfigMap as volume
- Zone serial stored in ConfigMap annotation

## Key Migration Considerations

### 1. Zone File Management

**Current:** Direct file system access to zone files
**Target:** ConfigMap-based zone files

**Migration:**
- Store zone files in Kubernetes ConfigMaps
- One ConfigMap per zone
- CoreDNS mounts ConfigMap as volume
- Update ConfigMap instead of writing files
- Maintain zone file format in ConfigMap

### 2. Serial Number Management

**Current:** Timestamp-based serial in zone files
**Target:** Serial in ConfigMap annotation or separate resource

**Migration:**
- Store zone serial in ConfigMap annotation
- Use monotonically increasing serials
- Handle serial updates atomically
- Coordinate serial updates across records

### 3. Record Aggregation

**Current:** Individual record operations on zone files
**Target:** Zone-level aggregation in controller

**Migration:**
- Controller aggregates all DNSRecords for a zone
- Generate complete zone file from all records
- Update ConfigMap atomically with all records
- Handle record additions/removals in aggregation

### 4. Corefile Generation

**Current:** Template-based Corefile generation
**Target:** ConfigMap-based Corefile

**Migration:**
- Store Corefile in ConfigMap
- Generate Corefile from domain configurations
- CoreDNS mounts Corefile ConfigMap
- Update Corefile when domains change

### 5. Record Validation

**Current:** Validation in record service
**Target:** CRD validation and webhook validation

**Migration:**
- Move validation to CRD OpenAPI schema
- Use validating webhooks for complex validation
- Maintain business logic validation in controller
- Remove service-level validation

### 6. State Reconstruction

**Current:** Zone files as source of truth
**Target:** CRDs as source of truth

**Migration:**
- CRDs are authoritative source
- Reconstruct zone files from CRDs on startup
- Handle orphaned zone file entries
- Migration tool for existing zone files

## Testing Strategy

### Current Testing Approach
- Unit tests for record operations
- Zone file parsing tests
- Validation tests
- Integration tests with file system

### Target Testing Approach
- Controller unit tests with fake client
- ConfigMap generation tests
- Zone file format tests
- Aggregation logic tests
- Integration tests with testenv

## Summary

The DNS domain manages DNS records and zone files with CoreDNS integration. Migration to Kubernetes will:

1. **DNSRecord CRD** - One CRD per DNS record
2. **ConfigMap Zone Files** - Zone files stored in ConfigMaps
3. **Zone Aggregation** - Controller aggregates records by zone
4. **ConfigMap Corefile** - Corefile in ConfigMap
5. **Serial Management** - Zone serials in ConfigMap annotations
6. **State Reconstruction** - Rebuild zone files from CRDs

The domain's validation, normalization, and zone file management logic should be preserved in the controller implementation, adapted for ConfigMap-based storage.

