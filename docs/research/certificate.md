# Certificate Domain Reference Architecture

## Executive Summary

The Certificate domain (`reference/internal/certificate/`) manages SSL/TLS certificate provisioning, renewal, and SAN (Subject Alternative Name) management using Let's Encrypt ACME protocol with DNS-01 challenges via Cloudflare. The domain handles automatic certificate lifecycle management with proactive cleanup and validation.

## Architecture Overview

### Current Architecture Pattern

The Certificate domain follows a **manager-based lifecycle pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Certificate Manager                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ ACME Client  │  │ DomainStorage │  │ DNS Provider │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │ SAN Manager  │  │ Process Mgr   │                        │
│  └──────────────┘  └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Let's Encrypt ACME integration via `go-acme/lego`
- Cloudflare DNS-01 challenge provider
- File-based certificate storage
- Automatic renewal with backoff
- SAN domain management
- Proactive DNS record cleanup

## Core Components

### 1. Certificate Manager

**Location:** `internal/certificate/manager.go`

**Responsibilities:**
- Certificate issuance via ACME
- Certificate renewal and expiration monitoring
- SAN domain management
- Certificate storage and retrieval
- TLS configuration integration

**Key Operations:**
```go
ObtainCertificate(domain string) error
AddDomainToSAN(domain string) error
RemoveDomainFromSAN(domain string) error
ValidateAndUpdateSANDomains() error
StartRenewalLoop(domain string)
GetCertificateInfo(domain string) (*CertificateInfo, error)
```

**State Management:**
- Certificate files: `/etc/letsencrypt/live/{domain}/`
- Domain storage: JSON file tracking base domain + SAN domains
- ACME user registration: JSON file
- Certificate info parsed from PEM files

### 2. Domain Storage

**Location:** `internal/certificate/domain_storage.go`

**Responsibilities:**
- Base domain and SAN domain tracking
- Domain persistence to JSON file
- Domain validation and management
- SAN list operations

**Key Operations:**
```go
AddDomain(domain string) error
RemoveDomain(domain string) error
GetBaseDomain() string
GetSANDomains() []string
GetAllDomains() []string
```

**Storage Format:**
```json
{
  "base_domain": "internal.example.test",
  "san_domains": [
    "app.internal.example.test",
    "api.internal.example.test"
  ]
}
```

### 3. DNS Record Adapter

**Location:** `internal/certificate/dns_record_adapter.go`

**Responsibilities:**
- Adapter between certificate manager and DNS record service
- Provides DNS record listing for SAN validation
- Enables certificate manager to query existing DNS records

**Key Operations:**
```go
ListRecords() ([]DNSRecord, error)
```

**Integration:**
- Connects certificate manager to DNS record service
- Enables automatic SAN validation against existing DNS records
- Supports certificate renewal when DNS records change

### 4. Process Manager

**Location:** `internal/certificate/process.go`

**Responsibilities:**
- Background certificate process management
- Certificate readiness signaling
- Retry logic with exponential backoff
- Process lifecycle coordination

**Key Operations:**
```go
NewProcessManager(dnsManager) (*ProcessManager, error)
StartWithRetry(timeout time.Duration) <-chan struct{}
GetManager() *Manager
```

**Features:**
- Retry logic for certificate provisioning
- Readiness channel for dependent services
- Background certificate renewal
- Error handling and logging

### 5. DNS Provider with Cleanup

**Location:** `internal/certificate/provider.go`

**Responsibilities:**
- Cloudflare DNS provider wrapper
- Proactive DNS record cleanup after ACME challenges
- DNS-01 challenge record management
- Cleanup error handling

**Key Operations:**
```go
NewCleaningDNSProvider(provider, token, zoneID) (*CleaningDNSProvider, error)
Present(domain, token, keyAuth) error
CleanUp(domain, token, keyAuth) error
```

**Cleanup Strategy:**
- Automatic cleanup of ACME challenge records
- Proactive cleanup on errors
- Zone ID discovery and management
- Error recovery and logging

### 6. SAN Management

**Location:** `internal/certificate/manager.go` (SAN methods)

**Responsibilities:**
- Automatic SAN domain addition/removal
- SAN validation against DNS records
- Certificate renewal on SAN changes
- Domain conflict detection

**Key Operations:**
```go
AddDomainToSAN(domain string) error
RemoveDomainFromSAN(domain string) error
ValidateAndUpdateSANDomains() error
```

**Validation Logic:**
- Checks if SAN domains have corresponding DNS records
- Removes invalid SAN domains
- Triggers certificate renewal when SAN list changes
- Handles edge cases (empty lists, duplicate domains)

## Data Flow

### Current Flow: Certificate Provisioning

```
1. ObtainCertificate(domain)
   ↓
2. ACME Client Registration (if needed)
   ↓
3. DNS-01 Challenge
   ├─→ Present() - Create TXT record via Cloudflare
   ├─→ Wait for propagation
   └─→ CleanUp() - Remove TXT record
   ↓
4. Certificate Issuance
   ↓
5. Certificate Storage
   ├─→ Save certificate files
   ├─→ Update domain storage
   └─→ Enable TLS in CoreDNS
   ↓
6. Start Renewal Loop
```

### Current Flow: SAN Management

```
1. AddDomainToSAN(domain)
   ↓
2. Validate domain exists in DNS records
   ↓
3. Add to SAN list in domain storage
   ↓
4. Trigger certificate renewal with new SAN list
   ↓
5. Update certificate files
```

## CRD Mapping Considerations

### Proposed CRD Structure

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
  autoRenew: true
  renewalThreshold: 30d
status:
  state: Ready
  certificateSecret:
    name: internal-cert-tls
    namespace: default
  expiresAt: "2024-12-31T23:59:59Z"
  conditions:
    - type: Ready
      status: "True"
    - type: Renewing
      status: "False"
```

### Reconciliation Logic

**CertificateController Responsibilities:**
- Watch Certificate CRDs
- Watch DNSRecord CRDs (for automatic SAN management)
- On create/update: Trigger ACME certificate request/renewal
- Monitor certificate expiration and auto-renew
- Store certificates in Kubernetes Secrets
- Update CoreDNS TLS configuration via ConfigMap
- Handle rate limiting and retries

**Automatic SAN Management:**
- When DNSRecord references a Certificate, automatically add DNSRecord FQDN to Certificate's SAN list
- When DNSRecord is deleted, automatically remove FQDN from Certificate's SAN list
- Trigger certificate renewal when SAN list changes

## Key Migration Considerations

### 1. Certificate Storage

**Current:** File-based storage (`/etc/letsencrypt/live/{domain}/`)
**Target:** Kubernetes Secrets

**Migration:**
- Store certificates as Kubernetes Secrets
- Use Secret labels for discovery
- Mount Secrets in CoreDNS pods
- Maintain file-based storage for compatibility if needed

### 2. Domain Storage

**Current:** JSON file tracking base domain + SAN domains
**Target:** CRD spec and status

**Migration:**
- Base domain in CRD spec
- SAN domains in CRD spec (with automatic management)
- Certificate state in CRD status
- Remove JSON file storage

### 3. ACME User Registration

**Current:** JSON file for ACME user registration
**Target:** Kubernetes Secret

**Migration:**
- Store ACME user registration in Secret
- Use Secret for ACME client initialization
- Maintain user registration across pod restarts

### 4. Renewal Loop

**Current:** Background goroutine with renewal loop
**Target:** Controller reconciliation with RequeueAfter

**Migration:**
- Use controller-runtime's RequeueAfter for renewal scheduling
- Monitor certificate expiration in reconciliation loop
- Trigger renewal when expiration threshold reached

### 5. DNS Provider Cleanup

**Current:** Proactive cleanup in DNS provider wrapper
**Target:** Maintain cleanup logic in certificate controller

**Migration:**
- Keep cleanup logic in certificate controller
- Ensure cleanup happens after ACME challenges
- Handle cleanup errors gracefully

### 6. Rate Limiting

**Current:** Exponential backoff in renewal logic
**Target:** Rate limit awareness in controller

**Migration:**
- Implement rate limit detection
- Use exponential backoff with RequeueAfter
- Handle Let's Encrypt rate limits gracefully

## Testing Strategy

### Current Testing Approach
- Unit tests for certificate operations
- Mock ACME client for testing
- SAN validation tests
- Edge case testing

### Target Testing Approach
- Controller unit tests with fake client
- Mock ACME provider for integration tests
- Secret management tests
- Rate limiting tests
- SAN management integration tests

## Summary

The Certificate domain manages SSL/TLS certificate lifecycle with Let's Encrypt and Cloudflare DNS challenges. Migration to Kubernetes will:

1. **Certificate CRD** - Declarative certificate management
2. **Kubernetes Secrets** - Certificate storage in Secrets
3. **Automatic SAN Management** - Controller automatically manages SAN domains based on DNSRecord references
4. **Controller-based Renewal** - Certificate renewal via controller reconciliation
5. **Rate Limit Handling** - Exponential backoff and rate limit awareness

The domain's proactive cleanup and SAN validation logic should be preserved in the controller implementation.

