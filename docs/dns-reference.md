# DNS Reference Architecture Research

## Executive Summary

This document examines the reference DNS service architecture as it relates to porting functionality to a Kubernetes CRD-Reconciler pattern using Kubebuilder and controller-runtime. The reference service is a unified containerized DNS management solution that provides automated DNS record management, SSL certificates, reverse proxy configuration, and Tailscale integration.

## Architectural Decisions

The following decisions have been made for the dns-operator implementation:

1. **One CRD per DNS Record** - Each DNS record will be represented as a separate DNSRecord CRD resource
2. **Separate Controllers with Boundaries** - Multiple focused controllers (DNSRecordController, CertificateController, ProxyRuleController, TailscaleDeviceController) with clear separation of concerns
3. **ConfigMap Zone Management** - DNS zone files will be managed via Kubernetes ConfigMaps rather than direct file system access
4. **Automatic SAN Management** - Certificate controller will automatically manage Subject Alternative Names when DNSRecords reference certificates
5. **ConfigMap Caddyfile** - Caddy reverse proxy configuration will be generated and stored in ConfigMaps
6. **TailscaleDevice CRD** - TailscaleDevice CRD will create references to Tailscale devices, but will NOT automatically create DNSRecord resources (manual DNSRecord creation required)

## Architecture Overview

### Current Architecture Pattern

The reference DNS service follows a **monolithic API-driven architecture**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Unified Container                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │   API    │  │ CoreDNS  │  │  Caddy   │  │  Cert    │   │
│  │ Service  │  │  Server  │  │  Proxy   │  │ Manager  │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘   │
│       │             │             │             │            │
│       └─────────────┴─────────────┴─────────────┘            │
│                    Supervisord                              │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Single unified container running multiple services via supervisord
- REST API as the primary interface for all operations
- File-based state management (zone files, config files, JSON storage)
- Synchronous request-response model
- Direct file system manipulation for DNS zones and proxy configs

### Target Architecture Pattern

The target architecture will follow **Kubernetes CRD-Reconciler pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│  ┌──────────────┐         ┌──────────────┐                  │
│  │ DNSRecord    │────────▶│ Reconciler   │                  │
│  │ CRD          │         │              │                  │
│  └──────────────┘         └──────┬───────┘                  │
│                                  │                           │
│                          ┌───────┴───────┐                   │
│                          │               │                   │
│                    ┌─────▼─────┐   ┌─────▼─────┐            │
│                    │ CoreDNS   │   │  Caddy    │            │
│                    │ Operator  │   │ Operator  │            │
│                    └───────────┘   └───────────┘            │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Declarative CRD-based resource definitions
- Event-driven reconciliation loops
- Kubernetes-native state management (etcd)
- Controller-runtime for resource watching and reconciliation
- Separation of concerns across multiple controllers

## Core Components Analysis

### 1. DNS Record Management

#### Current Implementation

**Location:** `internal/dns/record/service.go`, `internal/dns/coredns/manager.go`

**Responsibilities:**
- Create/update/delete DNS A records in zone files
- Manage zone file serial numbers
- Coordinate DNS record creation with proxy rules
- Validate and normalize DNS names
- Generate records from persisted zone files

**Key Operations:**
```go
// Service layer orchestrates DNS + Proxy
CreateRecord(req CreateRecordRequest, httpRequest ...*http.Request) (*Record, error)
RemoveRecord(req RemoveRecordRequest) error
ListRecords() ([]Record, error)

// CoreDNS manager handles zone file operations
AddRecord(serviceName, name, ip string) error
RemoveRecord(serviceName, name string) error
ListRecords(serviceName string) ([]Record, error)
```

**State Management:**
- Zone files stored on filesystem (`/etc/coredns/zones/{domain}.zone`)
- Zone file format: Standard DNS zone file with SOA, NS, and A records
- Serial number updates on every record change
- Corefile template-based configuration generation

**CRD Mapping Considerations:**

**Proposed CRD Structure:**
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
  proxy:
    enabled: true
    targetPort: 8080
    protocol: http
status:
  fqdn: app.internal.example.test
  conditions:
    - type: Ready
      status: "True"
```

**Reconciliation Logic:**
- Watch DNSRecord CRDs
- On create/update: Write to CoreDNS zone file via ConfigMap or direct file mount
- On delete: Remove from zone file
- Update status with current state
- Handle conflicts and retries

**Challenges:**
1. **Zone File Management:** Need to coordinate multiple DNSRecord resources writing to the same zone file
2. **Serial Number Management:** Zone serials must be monotonically increasing across all records
3. **Atomic Updates:** Multiple records in same zone need atomic updates
4. **State Reconstruction:** Need to rebuild zone file state from CRD resources on startup

**Solutions (DECIDED):**
- **ConfigMap-based Zone Management:** Store zone files in Kubernetes ConfigMaps
- Use a single reconciler per zone that aggregates all DNSRecord resources for that zone
- Store zone serial in ConfigMap annotation or separate ZoneStatus resource
- Use owner references to track which DNSRecord owns which zone file entry
- DNSRecordController will watch all DNSRecords and update the appropriate zone ConfigMap

### 2. Certificate Management

#### Current Implementation

**Location:** `internal/certificate/manager.go`

**Responsibilities:**
- Let's Encrypt certificate provisioning via DNS-01 challenges
- Certificate renewal and expiration monitoring
- SAN (Subject Alternative Name) management for multi-domain certificates
- Cloudflare DNS challenge integration
- Certificate storage and TLS configuration

**Key Operations:**
```go
ObtainCertificate(domain string) error
AddDomainToSAN(domain string) error
RemoveDomainFromSAN(domain string) error
ValidateAndUpdateSANDomains() error
StartRenewalLoop(domain string)
```

**State Management:**
- Certificate files stored on filesystem (`/etc/letsencrypt/live/{domain}/`)
- Domain storage in JSON file tracking base domain + SAN domains
- ACME user registration persisted to files
- Certificate info parsed from PEM files

**CRD Mapping Considerations:**

**Proposed CRD Structure:**
```yaml
apiVersion: certificate.jerkytreats.dev/v1alpha1
kind: Certificate
metadata:
  name: internal-cert
spec:
  baseDomain: internal.example.test
  provider: letsencrypt
  challengeType: dns01
  cloudflare:
    apiTokenSecretRef:
      name: cloudflare-token
      key: token
  sanDomains:
    - app.internal.example.test
    - api.internal.example.test
status:
  state: Ready
  certificateSecret:
    name: internal-cert-tls
    namespace: default
  expiresAt: "2024-12-31T23:59:59Z"
  conditions:
    - type: Ready
      status: "True"
```

**Reconciliation Logic:**
- Watch Certificate CRDs
- On create/update: Trigger ACME certificate request/renewal
- Monitor certificate expiration and auto-renew
- Store certificates in Kubernetes Secrets
- Update CoreDNS TLS configuration via ConfigMap or separate controller
- Handle rate limiting and retries

**Challenges:**
1. **SAN Management:** Multiple DNSRecord resources may need the same certificate
2. **Certificate Sharing:** Need to determine when to create new cert vs add to existing
3. **Rate Limiting:** Let's Encrypt has strict rate limits
4. **Secret Management:** Certificates stored as Kubernetes Secrets

**Solutions (DECIDED):**
- **Automatic SAN Management:** CertificateController will automatically manage SAN domains
- DNSRecord can reference a Certificate via owner reference or label selector
- When a DNSRecord references a Certificate, the CertificateController automatically adds the DNSRecord's FQDN to the certificate's SAN list
- When a DNSRecord is deleted, the CertificateController automatically removes the FQDN from the certificate's SAN list
- Use exponential backoff and rate limit awareness
- Store certificates in Secrets with proper labels for discovery
- Certificate renewal triggered automatically when SAN list changes

### 3. Reverse Proxy Management

#### Current Implementation

**Location:** `internal/proxy/manager.go`

**Responsibilities:**
- Caddy reverse proxy rule management
- Template-based Caddyfile generation
- Proxy rule persistence to JSON storage
- Automatic proxy setup for DNS records with ports
- Caddy configuration reload via supervisord

**Key Operations:**
```go
AddRule(proxyRule *ProxyRule) error
RemoveRule(hostname string) error
ListRules() []*ProxyRule
RestoreFromStorage() error
```

**State Management:**
- Proxy rules stored in JSON file (`data/proxy-rules.json`)
- Caddyfile generated from template with all active rules
- Configuration reloaded via supervisord commands

**CRD Mapping Considerations:**

**Option 1: Separate ProxyRule CRD**
```yaml
apiVersion: proxy.jerkytreats.dev/v1alpha1
kind: ProxyRule
metadata:
  name: webapp-proxy
spec:
  hostname: app.internal.example.test
  targetIP: 100.64.1.5
  targetPort: 8080
  protocol: http
status:
  state: Active
```

**Option 2: Embedded in DNSRecord (Current Pattern)**
```yaml
# DNSRecord with embedded proxy config
spec:
  proxy:
    enabled: true
    targetPort: 8080
    protocol: http
```

**Reconciliation Logic:**
- Watch ProxyRule CRDs (or DNSRecord with proxy enabled)
- Generate Caddyfile from template with all active rules
- Update Caddy ConfigMap with generated Caddyfile
- Trigger Caddy reload (via admin API or sidecar container)
- Handle rule conflicts and validation

**Challenges:**
1. **Caddy Integration:** Need to run Caddy as a separate pod or sidecar
2. **Config Reload:** Caddy admin API or file watching mechanism
3. **Rule Coordination:** Multiple ProxyRules need to be combined into single Caddyfile
4. **State Persistence:** Rules need to survive pod restarts

**Solutions (DECIDED):**
- **ConfigMap Caddyfile:** Store generated Caddyfile in Kubernetes ConfigMap
- Run Caddy as a DaemonSet or Deployment with ConfigMap mount
- Use Caddy's file-based config with reload plugin watching the ConfigMap
- Single ProxyRuleController that aggregates all ProxyRules into one Caddyfile ConfigMap
- Store proxy rules as CRDs (Kubernetes-native persistence)

### 4. Tailscale Integration

#### Current Implementation

**Location:** `internal/tailscale/client.go`, `internal/tailscale/sync/manager.go`

**Responsibilities:**
- Tailscale API client for device discovery
- Device IP resolution (100.64.x.x range)
- Automatic DNS record sync for Tailscale devices
- Device annotation and metadata management
- Polling-based device synchronization

**Key Operations:**
```go
ListDevices() ([]Device, error)
GetCurrentDeviceIP() (string, error)
GetDeviceByIP(ip string) (*Device, error)
EnsureInternalZone() error
RefreshDeviceIPs() error
```

**State Management:**
- Device data persisted in JSON file (`data/devices.json`)
- IP cache in memory for change detection
- Device annotations stored separately from Tailscale API data

**CRD Mapping Considerations:**

**Proposed CRD Structure:**
```yaml
apiVersion: tailscale.jerkytreats.dev/v1alpha1
kind: TailscaleDevice
metadata:
  name: my-device
spec:
  hostname: my-device
  autoSync: true
  annotations:
    description: "My development machine"
status:
  tailscaleIP: 100.64.1.5
  online: true
  lastSyncedAt: "2024-01-01T12:00:00Z"
```

**Reconciliation Logic:**
- Watch TailscaleDevice CRDs
- Poll Tailscale API periodically (configurable interval)
- **DO NOT automatically create DNSRecord resources** - users must create DNSRecords manually
- Update TailscaleDevice status with current IP, online state, and device metadata
- Provide device information for DNSRecord reconciliation (via reference or label selector)
- Handle device removal and IP changes

**Challenges:**
1. **API Rate Limiting:** Tailscale API has rate limits
2. **Polling vs Webhooks:** Current implementation uses polling
3. **Device Discovery:** How to discover new devices automatically
4. **IP Change Detection:** Need to update DNSRecord resources when device IPs change

**Solutions (DECIDED):**
- **TailscaleDevice CRD creates references only** - no automatic DNSRecord creation
- DNSRecord can reference a TailscaleDevice via owner reference or label selector
- DNSRecordController will resolve TailscaleDevice IPs when reconciling DNSRecords
- Use controller-runtime's RequeueAfter for polling intervals
- Implement exponential backoff for rate limit handling
- Use Tailscale webhooks if available, fallback to polling
- Use status subresource to track IP changes and expose device information
- DNSRecord reconciliation will watch referenced TailscaleDevices and update DNS records when device IPs change

### 5. Configuration Management

#### Current Implementation

**Location:** `internal/config/config.go`

**Responsibilities:**
- YAML configuration file loading via Viper
- Environment variable support
- Required key validation
- Configuration hot-reload capability
- Default value management

**Key Operations:**
```go
GetString(key string) string
GetInt(key string) int
GetBool(key string) bool
CheckRequiredKeys() error
Reload() error
```

**State Management:**
- Single YAML config file (`config.yaml`)
- Viper-based configuration with environment variable overrides
- Thread-safe singleton pattern

**CRD Mapping Considerations:**

**Kubernetes-Native Configuration:**
- Use ConfigMaps for non-sensitive configuration
- Use Secrets for sensitive data (API keys, tokens)
- Use CRD spec for resource-specific configuration
- Use operator-level ConfigMap for global settings

**Migration Strategy:**
- Convert YAML config to ConfigMap
- Move secrets to Kubernetes Secrets
- Embed resource-specific config in CRD specs
- Use controller-runtime's client for ConfigMap/Secret access

### 6. Persistence Layer

#### Current Implementation

**Location:** `internal/persistence/file.go`

**Responsibilities:**
- File-based storage with atomic writes
- Backup creation and management
- Thread-safe read/write operations
- Recovery from backup on corruption

**State Management:**
- JSON files for device storage (`data/devices.json`)
- JSON files for proxy rules (`data/proxy-rules.json`)
- Backup files with timestamps
- Atomic write operations via temp files

**CRD Mapping Considerations:**

**Kubernetes-Native Persistence:**
- CRDs stored in etcd (Kubernetes-native)
- No need for file-based persistence
- Use CRD status subresource for computed state
- Use finalizers for cleanup coordination

**Migration Benefits:**
- Automatic persistence via etcd
- Built-in versioning and history
- No manual backup/restore needed
- Distributed and highly available

## Data Flow Analysis

### Current Flow: API Request → DNS Record Creation

```
1. HTTP POST /add-record
   ↓
2. RecordHandler.AddRecord()
   ↓
3. RecordService.CreateRecord()
   ├─→ Validator.ValidateCreateRequest()
   ├─→ DNSManager.AddRecord() → Write zone file
   ├─→ ProxyManager.AddRule() → Generate Caddyfile
   └─→ CertificateManager.AddDomainToSAN() → Trigger cert renewal
   ↓
4. HTTP 201 Created
```

### Target Flow: CRD Creation → Reconciliation

```
1. kubectl apply -f dnsrecord.yaml
   ↓
2. Kubernetes API Server → etcd
   ↓
3. DNSRecord Controller (Watch)
   ├─→ Reconcile() triggered
   ├─→ Validate DNSRecord spec
   ├─→ Update CoreDNS ConfigMap/zone file
   ├─→ Create/update ProxyRule if proxy enabled
   └─→ Update DNSRecord status
   ↓
4. Status reflects current state
```

## State Management Comparison

### Current: File-Based State

**Pros:**
- Simple and direct
- Easy to inspect and debug
- No external dependencies

**Cons:**
- Not distributed
- Manual backup/restore
- Race conditions possible
- No built-in versioning
- Difficult to coordinate across instances

### Target: CRD-Based State

**Pros:**
- Distributed and highly available (etcd)
- Automatic versioning and history
- Built-in conflict resolution
- Kubernetes-native
- Easy to query and filter

**Cons:**
- Requires Kubernetes cluster
- etcd dependency
- More complex setup
- Learning curve for CRD patterns

## Key Migration Considerations

### 1. Resource Granularity

**Decision Point:** How to model DNS records as CRDs?

**Options:**
- **One CRD per DNS record:** Fine-grained, matches current API model
- **One CRD per zone:** Coarse-grained, manages all records in zone
- **Hybrid:** DNSRecord CRD with zone-level coordination

**DECISION:** One CRD per DNS record (DNSRecord) with zone-level coordination via ConfigMap aggregation.

### 2. Controller Architecture

**Decision Point:** Single controller vs multiple controllers?

**Options:**
- **Monolithic Controller:** One controller handles DNS, Proxy, Certificates
- **Separate Controllers:** DNSController, ProxyController, CertificateController
- **Hybrid:** Core controller with helper controllers

**DECISION:** Separate controllers with clear boundaries:
- **DNSRecordController:** Manages DNS records and zone ConfigMaps
- **CertificateController:** Manages Let's Encrypt certificates with automatic SAN management
- **ProxyRuleController:** Manages Caddy proxy rules and Caddyfile ConfigMap
- **TailscaleDeviceController:** Manages Tailscale device sync and provides device references (no automatic DNSRecord creation)

### 3. Zone File Management

**Challenge:** Multiple DNSRecord resources need to write to the same zone file.

**Solutions:**
1. **Zone-level Controller:** One controller per zone that aggregates all DNSRecords
2. **ConfigMap-based:** Store zone file in ConfigMap, reconcile all records together
3. **Finalizers:** Use finalizers to coordinate zone file updates

**DECISION:** ConfigMap-based approach with zone-level aggregation in DNSRecordController.
- Each zone has a corresponding ConfigMap (e.g., `zone-{domain}`)
- DNSRecordController watches all DNSRecords and aggregates them by zone
- Zone ConfigMap is updated atomically with all records for that zone
- CoreDNS mounts the ConfigMap as a volume for zone file access

### 4. Certificate Sharing

**Challenge:** Multiple DNSRecords may need the same certificate.

**Solutions:**
1. **Certificate CRD:** Separate Certificate resource that DNSRecords reference
2. **Automatic Pooling:** Controller automatically groups domains into certificates
3. **Manual Assignment:** Users explicitly create and reference certificates

**DECISION:** Certificate CRD with automatic SAN management.
- DNSRecord can reference a Certificate via owner reference or label selector
- CertificateController automatically adds DNSRecord FQDN to Certificate's SAN list when DNSRecord is created
- CertificateController automatically removes DNSRecord FQDN from Certificate's SAN list when DNSRecord is deleted
- Certificate renewal triggered automatically when SAN list changes

### 5. Proxy Rule Coordination

**Challenge:** Multiple ProxyRules need to be combined into a single Caddyfile.

**Solutions:**
1. **Aggregating Controller:** Single controller that watches all ProxyRules
2. **ConfigMap Generation:** Generate Caddyfile ConfigMap from all ProxyRules
3. **Caddy Admin API:** Use Caddy's dynamic config API

**DECISION:** ConfigMap-based Caddyfile generation with aggregating ProxyRuleController.
- ProxyRuleController watches all ProxyRule CRDs
- Generates single Caddyfile from all active ProxyRules
- Stores generated Caddyfile in ConfigMap (e.g., `caddy-config`)
- Caddy Deployment/DaemonSet mounts ConfigMap and watches for changes

### 6. Tailscale Integration

**Challenge:** How to model Tailscale devices and sync behavior.

**Solutions:**
1. **TailscaleDevice CRD:** Explicit device resources
2. **Automatic Discovery:** Controller automatically creates DNSRecords from Tailscale API
3. **Hybrid:** TailscaleDevice CRD with automatic DNSRecord creation
4. **Reference Only:** TailscaleDevice CRD provides device information, DNSRecords reference devices

**DECISION:** TailscaleDevice CRD that creates references to Tailscale devices, but does NOT automatically create DNSRecord resources.
- TailscaleDeviceController polls Tailscale API and updates TailscaleDevice CRD status
- DNSRecord can reference a TailscaleDevice via owner reference or label selector
- DNSRecordController resolves TailscaleDevice IP when reconciling DNSRecords
- Users must manually create DNSRecord resources (no automatic creation)
- DNSRecord reconciliation watches referenced TailscaleDevices and updates DNS records when device IPs change

## API Endpoint Mapping

### Current REST API → CRD Operations

| REST Endpoint | Method | CRD Equivalent |
|--------------|--------|---------------|
| `/add-record` | POST | `kubectl apply -f dnsrecord.yaml` |
| `/list-records` | GET | `kubectl get dnsrecords` |
| `/remove-record` | DELETE | `kubectl delete dnsrecord <name>` |
| `/list-devices` | GET | `kubectl get tailscaledevices` |
| `/annotate-device` | POST | `kubectl patch tailscaledevice <name>` |
| `/health` | GET | Controller health endpoint or Kubernetes probes |

**Note:** The REST API can still be provided as a convenience layer on top of CRDs using an API server or webhook.

## Testing Strategy

### Current Testing Approach

- Unit tests for individual components
- Integration tests with file system mocks
- Manual testing via API endpoints

### Target Testing Approach

- Unit tests with controller-runtime fake client
- Integration tests with testenv (controller-runtime test environment)
- End-to-end tests with kind/minikube clusters
- CRD validation tests
- Webhook validation tests

## Migration Phases

### Phase 1: Core DNS Record Management
- Create DNSRecord CRD
- Implement DNSRecordController
- Migrate zone file management to ConfigMap
- Basic create/update/delete operations

### Phase 2: Certificate Management
- Create Certificate CRD
- Implement CertificateController
- Integrate with DNSRecord for SAN management
- Certificate renewal and monitoring

### Phase 3: Proxy Management
- Create ProxyRule CRD (or extend DNSRecord)
- Implement ProxyRuleController
- Caddy integration and config generation
- Proxy rule coordination

### Phase 4: Tailscale Integration
- Create TailscaleDevice CRD
- Implement TailscaleDeviceController
- Automatic DNSRecord creation
- Device sync and polling

### Phase 5: Advanced Features
- Health checks and monitoring
- Metrics and observability
- Webhook validation
- RBAC and security policies

## Open Questions

1. **Zone File Format:** Should we maintain zone file format in ConfigMap or move to a more structured format (e.g., structured data with zone file generation)?

2. **Caddy Deployment:** Should Caddy run as a sidecar, DaemonSet, or separate Deployment?

3. **Certificate Storage:** Should certificates be stored in Secrets or separate storage mechanism?

4. **API Compatibility:** Should we maintain REST API compatibility layer or require users to use kubectl/API directly?

5. **Multi-Tenancy:** How to handle namespace isolation and resource quotas?

6. **Backup/Recovery:** How to handle etcd backup/restore for CRD state?

7. **DNSRecord-TailscaleDevice Binding:** What is the preferred method for DNSRecord to reference TailscaleDevice (owner reference, label selector, or explicit reference field)?

## Summary of Architectural Decisions

The following key decisions have been finalized for the dns-operator implementation:

| Decision Area | Decision | Rationale |
|--------------|----------|-----------|
| **DNS Record Granularity** | One CRD per DNS Record | Fine-grained control, matches current API model, enables per-record management |
| **Controller Architecture** | Separate controllers with clear boundaries | Separation of concerns, independent scaling, easier testing and maintenance |
| **Zone File Management** | ConfigMap-based zone files | Kubernetes-native, atomic updates, version control, easy to mount in CoreDNS |
| **Certificate SAN Management** | Automatic SAN management | Reduces manual certificate management, automatic renewal when DNSRecords change |
| **Proxy Configuration** | ConfigMap-based Caddyfile | Kubernetes-native, version control, easy to mount in Caddy, atomic updates |
| **Tailscale Integration** | TailscaleDevice CRD with references only | Explicit control, no automatic DNSRecord creation, users manage DNSRecords manually |

### Controller Responsibilities

- **DNSRecordController:** Watches DNSRecord CRDs, aggregates by zone, updates zone ConfigMaps
- **CertificateController:** Watches Certificate and DNSRecord CRDs, manages SAN lists automatically, handles certificate renewal
- **ProxyRuleController:** Watches ProxyRule CRDs, generates Caddyfile ConfigMap from all active rules
- **TailscaleDeviceController:** Polls Tailscale API, updates TailscaleDevice CRD status, provides device references (no DNSRecord creation)

## Conclusion

The reference DNS service provides a solid foundation for understanding the domain logic and requirements. The migration to a CRD-Reconciler pattern will require:

1. **Architectural Changes:**
   - Move from API-driven to declarative CRD model
   - Replace file-based state with Kubernetes resources
   - Implement event-driven reconciliation loops

2. **Component Separation:**
   - Split monolithic service into focused controllers
   - Clear boundaries between DNS, Certificate, Proxy, and Tailscale concerns
   - Use Kubernetes primitives (ConfigMaps, Secrets) for configuration

3. **State Management:**
   - Leverage etcd for distributed state
   - Use CRD status subresources for computed state
   - Implement finalizers for cleanup coordination

4. **Integration Points:**
   - CoreDNS integration via ConfigMaps or direct file mounts
   - Caddy integration via ConfigMaps or admin API
   - Tailscale API integration with proper rate limiting

The reference implementation provides excellent domain knowledge and business logic that can be directly ported to the reconciler pattern, with the main changes being in how state is managed and how operations are triggered (API calls → CRD watches).
