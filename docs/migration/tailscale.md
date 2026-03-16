# Tailscale Migration Guide

## Overview

This guide covers the migration of the Tailscale integration from the current standalone implementation to a Kubernetes operator-based architecture using Custom Resource Definitions (CRDs).

### What's Changing

- **Device Discovery**: From polling-based sync manager to controller-based reconciliation with `RequeueAfter`
- **Device Storage**: From JSON file storage (`data/devices.json`) to CRD storage in etcd
- **DNS Record Creation**: From automatic creation to manual DNSRecord resource creation
- **Device Management**: From HTTP endpoints to Kubernetes CRD operations
- **IP Resolution**: From client methods to controller-based resolution via DNSRecord references

## Architecture Comparison

### Current Architecture

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

**Key Components:**
- `Client` - Tailscale API communication
- `Sync Manager` - Polling-based device synchronization
- `Device Handler` - HTTP endpoints for device management
- `File Storage` - JSON file persistence

### Target Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Operator                       │
│  ┌──────────────────┐  ┌──────────────────┐                │
│  │ TailscaleDevice  │  │ DNSRecord        │                │
│  │ Controller       │  │ Controller       │                │
│  └──────────────────┘  └──────────────────┘                │
│  ┌──────────────────┐  ┌──────────────────┐                │
│  │ TailscaleDevice  │  │ DNSRecord        │                │
│  │ CRD              │  │ CRD              │                │
│  └──────────────────┘  └──────────────────┘                │
└─────────────────────────────────────────────────────────────┘
```

**Key Components:**
- `TailscaleDeviceController` - Reconciles TailscaleDevice CRDs
- `DNSRecordController` - Reconciles DNSRecord CRDs with TailscaleDevice references
- `TailscaleDevice CRD` - Kubernetes resource for device representation
- `DNSRecord CRD` - Kubernetes resource for DNS records

## Migration Steps

### Step 1: Understand Current Configuration

**Current Configuration Keys:**
- `tailscale.api_key` - Tailscale API key
- `tailscale.tailnet` - Tailscale tailnet name
- `tailscale.base_url` - Tailscale API base URL (default: `https://api.tailscale.com`)
- `tailscale.device_name` - Optional device name

**Current Storage:**
- JSON file: `data/devices.json`
- Contains device list with metadata and annotations

**Current Endpoints:**
- `GET /list-devices` - List all Tailscale devices
- `POST /annotate-device` - Update device annotations

### Step 2: Create TailscaleDevice CRD

Create the `TailscaleDevice` CRD definition:

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tailscaledevices.tailscale.jerkytreats.dev
spec:
  group: tailscale.jerkytreats.dev
  versions:
  - name: v1alpha1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              hostname:
                type: string
              autoSync:
                type: boolean
                default: true
              annotations:
                type: object
                additionalProperties:
                  type: string
          status:
            type: object
            properties:
              tailscaleIP:
                type: string
              online:
                type: boolean
              lastSyncedAt:
                type: string
                format: date-time
              conditions:
                type: array
                items:
                  type: object
                  properties:
                    type:
                      type: string
                    status:
                      type: string
                    message:
                      type: string
  scope: Namespaced
  names:
    plural: tailscaledevices
    singular: tailscaledevice
    kind: TailscaleDevice
```

### Step 3: Implement TailscaleDevice Controller

**Controller Responsibilities:**
1. Poll Tailscale API periodically using `RequeueAfter`
2. Update TailscaleDevice status with current IP, online state, and metadata
3. Handle rate limiting with exponential backoff
4. **DO NOT** automatically create DNSRecord resources

**Key Implementation Points:**

```go
// Polling interval using RequeueAfter
func (r *TailscaleDeviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Poll Tailscale API
    devices, err := r.tailscaleClient.ListDevices()
    if err != nil {
        // Handle rate limiting with exponential backoff
        return ctrl.Result{RequeueAfter: backoffDuration}, err
    }
    
    // Update TailscaleDevice status
    device.Status.TailscaleIP = getTailscaleIP(device)
    device.Status.Online = device.Online
    device.Status.LastSyncedAt = metav1.Now()
    
    // Requeue after polling interval
    return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

### Step 4: Update DNSRecord CRD for TailscaleDevice Reference

Add support for TailscaleDevice references in the DNSRecord CRD:

```yaml
spec:
  targetIP: string  # Direct IP (existing)
  tailscaleDevice:  # New: Reference to TailscaleDevice
    name: string
    namespace: string  # Optional, defaults to same namespace
```

**DNSRecord Controller Changes:**
- Watch TailscaleDevice resources
- Resolve TailscaleDevice IP when reconciling DNSRecords
- Update DNSRecord when referenced TailscaleDevice IP changes

### Step 5: Migrate Device Data

**Export Current Device Data:**

1. Access the current device storage file: `data/devices.json`
2. Extract device information:
   - Device names/hostnames
   - Annotations
   - Current IPs (will be refreshed by controller)

**Create TailscaleDevice Resources:**

For each device in your current storage, create a TailscaleDevice resource:

```yaml
apiVersion: tailscale.jerkytreats.dev/v1alpha1
kind: TailscaleDevice
metadata:
  name: my-device
  namespace: default
spec:
  hostname: my-device
  autoSync: true
  annotations:
    description: "My development machine"
    # ... other annotations from current storage
```

**Migration Script Example:**

```bash
#!/bin/bash
# Export devices from current storage and create TailscaleDevice CRDs

# Read current device storage
DEVICES=$(cat data/devices.json | jq -r '.devices[]')

# Create TailscaleDevice for each device
echo "$DEVICES" | jq -r '.name' | while read device_name; do
  kubectl apply -f - <<EOF
apiVersion: tailscale.jerkytreats.dev/v1alpha1
kind: TailscaleDevice
metadata:
  name: ${device_name}
spec:
  hostname: ${device_name}
  autoSync: true
EOF
done
```

### Step 6: Create DNSRecord Resources

**Important:** DNSRecord resources must be created manually. The TailscaleDevice controller does NOT automatically create DNSRecords.

**Option 1: Direct IP Reference (No Change)**

```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: webapp
spec:
  targetIP: 100.64.1.5
  # ... other DNSRecord fields
```

**Option 2: TailscaleDevice Reference (New)**

```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: webapp
spec:
  tailscaleDevice:
    name: my-device
    namespace: default  # Optional
  # ... other DNSRecord fields
```

**Benefits of TailscaleDevice Reference:**
- Automatic IP updates when device IP changes
- DNSRecord controller watches TailscaleDevice for changes
- No manual IP updates required

### Step 7: Update Configuration

**Remove HTTP Endpoints:**
- Remove `GET /list-devices` endpoint
- Remove `POST /annotate-device` endpoint
- Use `kubectl` commands instead

**Configuration Migration:**

**Current (Config File):**
```yaml
tailscale:
  api_key: "${TAILSCALE_API_KEY}"
  tailnet: "${TAILSCALE_TAILNET}"
  base_url: "https://api.tailscale.com"
  device_name: "optional-device-name"
```

**Target (Kubernetes Secret):**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tailscale-credentials
  namespace: dns-operator-system
type: Opaque
stringData:
  api-key: "${TAILSCALE_API_KEY}"
  tailnet: "${TAILSCALE_TAILNET}"
  base-url: "https://api.tailscale.com"
```

**Controller Configuration:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tailscale-config
  namespace: dns-operator-system
data:
  polling-interval: "5m"
  base-url: "https://api.tailscale.com"
```

### Step 8: Remove Old Components

**Components to Remove:**
1. `internal/tailscale/sync/manager.go` - Sync manager (replaced by controller)
2. `internal/tailscale/handler/handler.go` - HTTP handlers (replaced by kubectl)
3. Device persistence file storage - Replaced by CRD storage
4. Polling goroutines - Replaced by controller RequeueAfter

**Components to Keep:**
1. `internal/tailscale/client.go` - Tailscale API client (used by controller)
2. `internal/tailscale/device.go` - Device model (adapted for CRD)

## Key Migration Decisions

### 1. No Automatic DNSRecord Creation

**Decision:** TailscaleDevice CRD does NOT automatically create DNSRecord resources.

**Rationale:**
- Separation of concerns: Device discovery vs DNS record management
- User control: Users explicitly define DNS records
- Flexibility: Multiple DNS records can reference the same device

**Migration Impact:**
- Users must manually create DNSRecord resources
- DNSRecord can reference TailscaleDevice for automatic IP resolution

### 2. Controller-Based Polling

**Decision:** Use controller-runtime's `RequeueAfter` for polling intervals.

**Rationale:**
- Standard Kubernetes pattern
- Built-in backoff and retry mechanisms
- Better observability and debugging

**Migration Impact:**
- Replace `StartPolling()` goroutine with controller reconciliation
- Use `RequeueAfter` instead of ticker-based polling

### 3. CRD Storage

**Decision:** Replace JSON file storage with CRD storage in etcd.

**Rationale:**
- Native Kubernetes storage
- Built-in versioning and history
- Better integration with Kubernetes tooling

**Migration Impact:**
- Device data stored as TailscaleDevice CRDs
- No file system dependencies
- Use `kubectl` for device management

### 4. Explicit DNSRecord-TailscaleDevice Binding

**Decision:** Use explicit reference field in DNSRecord spec.

**Rationale:**
- Clear and explicit binding
- Easy to query and filter
- Supports both direct IP and device reference

**Migration Impact:**
- DNSRecord spec includes `tailscaleDevice` field
- DNSRecordController resolves TailscaleDevice IP
- Automatic updates when device IP changes

## Testing Strategy

### Unit Tests

**Current:**
- Client operation tests
- Mock Tailscale API responses
- Device sync tests

**Target:**
- Controller unit tests with fake client
- Mock Tailscale API for integration tests
- TailscaleDevice CRD tests
- DNSRecord-TailscaleDevice binding tests

### Integration Tests

**Test Scenarios:**
1. TailscaleDevice creation and status updates
2. DNSRecord with TailscaleDevice reference
3. IP change detection and DNSRecord updates
4. Rate limiting and backoff handling
5. Device removal and cleanup

### Migration Testing

**Test Checklist:**
- [ ] Device data successfully migrated to TailscaleDevice CRDs
- [ ] DNSRecord resources created with TailscaleDevice references
- [ ] Controller polling working correctly
- [ ] IP changes detected and DNSRecords updated
- [ ] Old HTTP endpoints removed
- [ ] Configuration migrated to Kubernetes Secrets/ConfigMaps

## Rollback Plan

If migration issues occur:

1. **Keep Old System Running:**
   - Don't remove old components immediately
   - Run both systems in parallel during migration

2. **Data Backup:**
   - Backup `data/devices.json` before migration
   - Export TailscaleDevice CRDs after migration

3. **Rollback Steps:**
   - Restore old configuration
   - Restore device storage file
   - Re-enable old HTTP endpoints
   - Remove TailscaleDevice CRDs

## Post-Migration Tasks

1. **Monitor Controller Logs:**
   ```bash
   kubectl logs -n dns-operator-system deployment/tailscale-controller -f
   ```

2. **Verify Device Sync:**
   ```bash
   kubectl get tailscaledevices -o wide
   kubectl describe tailscaledevice <device-name>
   ```

3. **Verify DNSRecord Updates:**
   ```bash
   kubectl get dnsrecords -o wide
   kubectl describe dnsrecord <record-name>
   ```

4. **Clean Up:**
   - Remove old device storage file
   - Remove old HTTP endpoint code
   - Remove old sync manager code

## Common Issues and Solutions

### Issue: Devices Not Syncing

**Symptoms:** TailscaleDevice status not updating

**Solutions:**
- Check Tailscale API credentials in Secret
- Verify controller is running and has permissions
- Check controller logs for errors
- Verify Tailscale API connectivity

### Issue: DNSRecord Not Updating

**Symptoms:** DNSRecord IP not matching TailscaleDevice IP

**Solutions:**
- Verify DNSRecord references correct TailscaleDevice
- Check DNSRecordController is watching TailscaleDevices
- Verify DNSRecordController has permissions
- Check DNSRecordController logs

### Issue: Rate Limiting

**Symptoms:** Controller errors with rate limit messages

**Solutions:**
- Increase polling interval in ConfigMap
- Implement exponential backoff (should be automatic)
- Check Tailscale API rate limits
- Consider reducing number of devices polled

## Additional Resources

- [Tailscale API Documentation](https://tailscale.com/kb/1242/api)
- [Kubernetes Controller Patterns](https://kubernetes.io/docs/concepts/architecture/controller/)
- [CRD Best Practices](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/)

