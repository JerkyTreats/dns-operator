# Tailscale Domain Reference Architecture

## Executive Summary

The Tailscale domain (`reference/internal/tailscale/`) provides integration with the Tailscale API for device discovery, IP resolution, and device synchronization. It includes client management, device handling, sync operations, and HTTP handlers for device management endpoints.

## Architecture Overview

### Current Architecture Pattern

The Tailscale domain follows a **client-based API integration pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Tailscale Domain                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Client       │  │ Device       │  │ Sync Manager │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │ Handler      │  │ Persistence  │                        │
│  └──────────────┘  └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Tailscale API client for device discovery
- Device IP resolution (100.64.x.x range)
- Polling-based device synchronization
- Device persistence and caching
- HTTP handlers for device management

## Core Components

### 1. Tailscale Client

**Location:** `internal/tailscale/client.go`

**Responsibilities:**
- Tailscale API communication
- Device listing and discovery
- IP resolution and device lookup
- Connection validation

**Key Operations:**
```go
NewClient() (*Client, error)
ListDevices() ([]Device, error)
GetDevice(nameOrHostname string) (*Device, error)
GetDeviceIP(deviceName string) (string, error)
GetDeviceByIP(ip string) (*Device, error)
GetCurrentDeviceIP() (string, error)
ValidateConnection() error
```

**Configuration:**
- API key: `tailscale.api_key`
- Tailnet: `tailscale.tailnet`
- Base URL: `tailscale.base_url` (default: `https://api.tailscale.com`)
- Device name: `tailscale.device_name` (optional)

### 2. Device Model

**Location:** `internal/tailscale/device.go`, `internal/tailscale/client.go`

**Responsibilities:**
- Device data representation
- Device metadata management
- IP address extraction

**Structure:**
```go
type Device struct {
    Name      string   // Device name
    Hostname  string   // Device hostname
    Addresses []string // IP addresses (100.64.x.x)
    Online    bool     // Online status
    ID        string   // Device ID
}
```

**Key Operations:**
```go
GetTailscaleIP(device *Device) string
GetDeviceByIP(ip string) (*Device, error)
```

### 3. Sync Manager

**Location:** `internal/tailscale/sync/manager.go`

**Responsibilities:**
- Automatic device synchronization
- DNS record creation for devices
- Polling-based sync
- Zone management

**Key Operations:**
```go
NewManager(dnsManager, tailscaleClient, deviceStorage) (*Manager, error)
EnsureInternalZone() error
RefreshDeviceIPs() error
StartPolling(interval time.Duration)
```

**Sync Flow:**
1. Poll Tailscale API for devices
2. Compare with stored devices
3. Create/update DNS records for devices
4. Update device storage
5. Repeat at configured interval

### 4. Device Handler

**Location:** `internal/tailscale/handler/handler.go`

**Responsibilities:**
- HTTP endpoints for device management
- Device listing
- Device annotation
- Device metadata updates

**Key Operations:**
```go
NewDeviceHandlerWithDefaults() (*DeviceHandler, error)
ListDevices(w http.ResponseWriter, r *http.Request)
AnnotateDevice(w http.ResponseWriter, r *http.Request)
```

**Endpoints:**
- `GET /list-devices` - List all Tailscale devices
- `POST /annotate-device` - Update device annotations

### 5. Device Persistence

**Location:** `internal/tailscale/sync/manager.go` (uses persistence package)

**Responsibilities:**
- Device data persistence
- Device storage management
- Backup and recovery

**Storage:**
- JSON file: `data/devices.json`
- Device list with metadata
- IP cache for change detection

## Data Flow

### Current Flow: Device Discovery

```
1. ListDevices()
   ↓
2. HTTP GET to Tailscale API
   ├─→ /api/v2/tailnet/{tailnet}/devices
   └─→ Authenticate with API key
   ↓
3. Parse device response
   ↓
4. Return device list
```

### Current Flow: Device Sync

```
1. StartPolling(interval)
   ↓
2. Poll Tailscale API
   ├─→ Get current devices
   └─→ Compare with stored devices
   ↓
3. Detect changes
   ├─→ New devices
   ├─→ IP changes
   └─→ Removed devices
   ↓
4. Update DNS records
   ├─→ Create records for new devices
   ├─→ Update records for IP changes
   └─→ Remove records for deleted devices
   ↓
5. Update device storage
   ↓
6. Wait for interval
   ↓
7. Repeat
```

## CRD Mapping Considerations

### Proposed CRD Structure

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
  conditions:
    - type: Ready
      status: "True"
```

### Reconciliation Logic

**TailscaleDeviceController Responsibilities:**
- Watch TailscaleDevice CRDs
- Poll Tailscale API periodically (configurable interval)
- **DO NOT automatically create DNSRecord resources** - users must create DNSRecords manually
- Update TailscaleDevice status with current IP, online state, and device metadata
- Provide device information for DNSRecord reconciliation (via reference or label selector)
- Handle device removal and IP changes

**DNSRecord Integration:**
- DNSRecord can reference a TailscaleDevice via owner reference or label selector
- DNSRecordController resolves TailscaleDevice IP when reconciling DNSRecords
- DNSRecord reconciliation watches referenced TailscaleDevices and updates DNS records when device IPs change

## Key Migration Considerations

### 1. Device Discovery

**Current:** Polling-based device discovery
**Target:** Controller-based polling with RequeueAfter

**Migration:**
- Use controller-runtime's RequeueAfter for polling intervals
- Poll Tailscale API in reconciliation loop
- Update TailscaleDevice CRD status
- Handle rate limiting with exponential backoff

### 2. Automatic DNS Record Creation

**Current:** Sync manager automatically creates DNS records
**Target:** Manual DNSRecord creation required

**Migration:**
- **DECISION:** TailscaleDevice CRD does NOT automatically create DNSRecord resources
- Users must manually create DNSRecord resources
- DNSRecord can reference TailscaleDevice for IP resolution
- DNSRecordController resolves TailscaleDevice IP when reconciling

### 3. Device Persistence

**Current:** JSON file for device storage
**Target:** CRD storage in etcd

**Migration:**
- Replace JSON storage with TailscaleDevice CRDs
- CRDs stored in etcd automatically
- Remove device storage file management
- Use CRD status for device state

### 4. IP Resolution

**Current:** Client methods for IP resolution
**Target:** Controller-based IP resolution

**Migration:**
- DNSRecordController resolves TailscaleDevice IP
- Use TailscaleDevice status for current IP
- Watch TailscaleDevice for IP changes
- Update DNSRecord when device IP changes

### 5. Rate Limiting

**Current:** Basic timeout handling
**Target:** Exponential backoff and rate limit awareness

**Migration:**
- Implement exponential backoff for rate limits
- Use RequeueAfter with backoff
- Handle Tailscale API rate limits gracefully
- Monitor rate limit headers if available

### 6. Device Annotations

**Current:** HTTP endpoint for device annotations
**Target:** CRD spec for device metadata

**Migration:**
- Move annotations to TailscaleDevice spec
- Use kubectl patch for updates
- Remove HTTP annotation endpoint
- Maintain annotation structure in CRD

## DNSRecord-TailscaleDevice Binding

### Reference Methods

**Option 1: Owner Reference**
```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: webapp
  ownerReferences:
  - apiVersion: tailscale.jerkytreats.dev/v1alpha1
    kind: TailscaleDevice
    name: my-device
```

**Option 2: Label Selector**
```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: webapp
  labels:
    tailscale.device: my-device
spec:
  tailscaleDeviceRef:
    name: my-device
```

**Option 3: Explicit Reference Field**
```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
spec:
  targetIP: 100.64.1.5  # Or
  tailscaleDevice:
    name: my-device
```

**Recommended Approach:**
- **Option 3** - Explicit reference field in DNSRecord spec
- Clear and explicit binding
- Easy to query and filter
- Supports both direct IP and device reference

## Testing Strategy

### Current Testing Approach
- Unit tests for client operations
- Mock Tailscale API responses
- Device sync tests
- IP resolution tests

### Target Testing Approach
- Controller unit tests with fake client
- Mock Tailscale API for integration tests
- TailscaleDevice CRD tests
- DNSRecord-TailscaleDevice binding tests
- IP change detection tests

## Summary

The Tailscale domain provides Tailscale API integration for device discovery and IP resolution. Migration to Kubernetes will:

1. **TailscaleDevice CRD** - CRD for Tailscale device representation
2. **Controller Polling** - Use RequeueAfter for polling intervals
3. **No Automatic DNSRecord Creation** - Users must manually create DNSRecord resources
4. **DNSRecord Reference** - DNSRecord can reference TailscaleDevice for IP resolution
5. **IP Change Detection** - DNSRecordController watches TailscaleDevice for IP changes
6. **CRD Storage** - Replace JSON storage with CRD storage

The domain's device discovery and IP resolution logic should be preserved in the controller implementation, with explicit separation between TailscaleDevice management and DNSRecord creation.


