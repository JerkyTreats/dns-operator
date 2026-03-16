# Proxy Domain Reference Architecture

## Executive Summary

The Proxy domain (`reference/internal/proxy/`) manages Caddy reverse proxy configuration with template-based Caddyfile generation, rule persistence, and automatic proxy setup for DNS records with ports. It provides dynamic proxy rule management with storage and restoration capabilities.

## Architecture Overview

### Current Architecture Pattern

The Proxy domain follows a **manager-based rule management pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Proxy Manager                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Rule Storage │  │ Caddyfile    │  │ Reloader      │      │
│  │              │  │ Generator    │  │              │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Template-based Caddyfile generation
- JSON-based rule persistence
- Automatic proxy rule creation
- Caddy configuration reload via supervisord
- Rule validation and management

## Core Components

### 1. Proxy Manager

**Location:** `internal/proxy/manager.go`

**Responsibilities:**
- Proxy rule lifecycle management
- Caddyfile generation from template
- Rule persistence and restoration
- Configuration reload coordination

**Key Operations:**
```go
NewManager(reloader Reloader) (*Manager, error)
AddRule(proxyRule *ProxyRule) error
RemoveRule(hostname string) error
ListRules() []*ProxyRule
RestoreFromStorage() error
IsEnabled() bool
```

**Configuration:**
- Config path: `proxy.config_path` (default: `/app/configs/Caddyfile`)
- Template path: `proxy.template_path` (default: `/etc/caddy/Caddyfile.template`)
- Enabled: `proxy.enabled` (default: true)

### 2. Proxy Rule

**Location:** `internal/proxy/manager.go` (ProxyRule struct)

**Responsibilities:**
- Proxy rule representation
- Rule validation
- Rule metadata

**Structure:**
```go
type ProxyRule struct {
    Hostname   string    // llm.internal.example.test
    TargetIP   string    // 100.2.2.2
    TargetPort int       // 8080
    Protocol   string    // http/https
    Enabled    bool
    CreatedAt  time.Time
}
```

**Validation:**
- FQDN validation for hostname
- IP address validation
- Port range validation (1-65535)
- Protocol validation (http/https)

### 3. Rule Storage

**Location:** `internal/proxy/storage.go`

**Responsibilities:**
- JSON-based rule persistence
- Rule restoration on startup
- Storage file management

**Key Operations:**
```go
SaveRules(rules []*ProxyRule) error
LoadRules() ([]*ProxyRule, error)
```

**Storage Format:**
- JSON file: `data/proxy-rules.json`
- Array of ProxyRule objects
- Timestamp tracking

### 4. Caddyfile Generation

**Location:** `internal/proxy/manager.go` (generateCaddyfile)

**Responsibilities:**
- Template-based Caddyfile generation
- Rule aggregation
- Configuration file writing

**Template Variables:**
- Rules: Array of proxy rules
- Domain configuration
- TLS settings

**Generation Flow:**
1. Load Caddyfile template
2. Aggregate all active rules
3. Execute template with rules
4. Write generated Caddyfile
5. Trigger Caddy reload

### 5. Configuration Reloader

**Location:** `internal/proxy/manager.go` (CaddyReloader)

**Responsibilities:**
- Caddy configuration reload
- Supervisord integration
- Reload coordination

**Key Operations:**
```go
Reload(configPath string) error
```

**Reload Process:**
1. `supervisorctl reread` - Reread configuration
2. `supervisorctl update` - Update and restart services
3. Caddy reloads with new configuration

## Data Flow

### Current Flow: Proxy Rule Creation

```
1. AddRule(proxyRule)
   ↓
2. Validate rule
   ↓
3. Add to in-memory rules
   ↓
4. Save rules to storage
   ↓
5. Generate Caddyfile
   ├─→ Load template
   ├─→ Aggregate all rules
   └─→ Write Caddyfile
   ↓
6. Reload Caddy configuration
   └─→ supervisord reload
```

### Current Flow: Caddyfile Generation

```
1. Generate Caddyfile
   ↓
2. Load template file
   ↓
3. Collect all active rules
   ↓
4. Execute template
   ├─→ Render rules
   └─→ Generate configuration
   ↓
5. Write to config path
   ↓
6. Trigger reload
```

## CRD Mapping Considerations

### Proposed CRD Structure

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

**Option 2: Embedded in DNSRecord**
```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: webapp
spec:
  name: webapp
  domain: internal.example.test
  proxy:
    enabled: true
    targetPort: 8080
    protocol: http
```

**Recommended Approach:**
- **Option 1** - Separate ProxyRule CRD
- Clear separation of concerns
- Independent proxy rule management
- Easier to extend with additional proxy features

### Reconciliation Logic

**ProxyRuleController Responsibilities:**
- Watch ProxyRule CRDs
- Aggregate all ProxyRules
- Generate Caddyfile from all active rules
- Update Caddyfile ConfigMap
- Trigger Caddy reload (via sidecar or admin API)
- Handle rule conflicts and validation

**Caddyfile Management:**
- Store generated Caddyfile in ConfigMap
- Single ConfigMap for all proxy rules
- Controller aggregates all ProxyRules into one Caddyfile
- Caddy Deployment/DaemonSet mounts ConfigMap

## Key Migration Considerations

### 1. Rule Storage

**Current:** JSON file for rule persistence
**Target:** CRD storage in etcd

**Migration:**
- Replace JSON storage with ProxyRule CRDs
- CRDs stored in etcd automatically
- No manual persistence needed
- Remove storage file management

### 2. Caddyfile Generation

**Current:** Template-based generation to file
**Target:** ConfigMap-based Caddyfile

**Migration:**
- Generate Caddyfile in controller
- Store in Kubernetes ConfigMap
- Caddy mounts ConfigMap as volume
- Update ConfigMap on rule changes

### 3. Configuration Reload

**Current:** Supervisord-based reload
**Target:** Caddy admin API or file watching

**Migration:**
- Use Caddy admin API for dynamic config (preferred)
- Or use file watching with ConfigMap
- Remove supervisord dependency
- Kubernetes-native reload mechanism

### 4. Rule Aggregation

**Current:** In-memory rule aggregation
**Target:** Controller-based aggregation

**Migration:**
- Controller watches all ProxyRules
- Aggregates rules in reconciliation loop
- Generates single Caddyfile from all rules
- Updates ConfigMap atomically

### 5. Caddy Deployment

**Current:** Caddy in unified container via supervisord
**Target:** Caddy as separate Deployment or DaemonSet

**Migration:**
- Run Caddy as separate pod
- Mount Caddyfile ConfigMap
- Use Caddy admin API or file watching
- Remove supervisord dependency

### 6. Rule Validation

**Current:** Validation in proxy manager
**Target:** CRD validation and webhook validation

**Migration:**
- Move validation to CRD OpenAPI schema
- Use validating webhooks for complex validation
- Maintain business logic validation in controller

## Caddyfile ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: caddy-config
data:
  Caddyfile: |
    app.internal.example.test {
        reverse_proxy 100.64.1.5:8080
    }
    api.internal.example.test {
        reverse_proxy 100.64.1.6:8080
    }
```

## Testing Strategy

### Current Testing Approach
- Unit tests for rule management
- Caddyfile generation tests
- Storage tests
- Validation tests

### Target Testing Approach
- Controller unit tests with fake client
- ConfigMap generation tests
- Caddyfile format tests
- Aggregation logic tests
- Integration tests with testenv

## Summary

The Proxy domain manages Caddy reverse proxy configuration with template-based generation. Migration to Kubernetes will:

1. **ProxyRule CRD** - Separate CRD for proxy rules
2. **ConfigMap Caddyfile** - Store generated Caddyfile in ConfigMap
3. **Controller Aggregation** - Controller aggregates all ProxyRules into one Caddyfile
4. **Caddy Deployment** - Run Caddy as separate Deployment/DaemonSet
5. **Admin API or File Watching** - Use Caddy admin API or ConfigMap file watching for reload
6. **Remove Storage** - Replace JSON storage with CRD storage

The domain's template-based generation and rule aggregation logic should be preserved in the controller implementation, adapted for ConfigMap-based storage and Kubernetes-native deployment.

