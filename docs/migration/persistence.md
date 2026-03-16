# Persistence Domain Migration Guide

## Overview

This guide covers migrating the Persistence domain from file-based storage to Kubernetes-native CRD storage. The current implementation uses file-based JSON storage with atomic operations, backup management, and recovery capabilities. The target architecture leverages Kubernetes CRDs stored in etcd, eliminating the need for application-level persistence management.

## Migration Summary

| Aspect | Current (File Storage) | Target (Kubernetes CRD) |
|--------|----------------------|------------------------|
| **Storage** | File-based JSON | CRD resources in etcd |
| **Backups** | Application-managed backups | Kubernetes versioning + etcd backups |
| **Atomicity** | Temporary file pattern | Kubernetes API atomicity |
| **Recovery** | Backup file recovery | Kubernetes resource recovery |
| **Concurrency** | Mutex-based locking | Resource version (optimistic concurrency) |
| **Thread Safety** | Manual mutex management | Kubernetes API handles concurrency |

## Pre-Migration Assessment

### Current Components to Migrate

1. **FileStorage** (`internal/persistence/file.go`)
   - File read/write operations
   - Atomic write operations
   - Backup management
   - Recovery operations

2. **Configuration Dependencies**
   - `device.storage.path` - File path configuration
   - `device.storage.backup_count` - Backup count (default: 3)

3. **Usage Points**
   - Identify all consumers of `FileStorage`
   - Map file storage operations to CRD operations
   - Identify data structures stored in files

### Migration Prerequisites

- [ ] Kubernetes cluster with etcd access
- [ ] CRD definitions created for stored data
- [ ] Controller-runtime setup for CRD operations
- [ ] Backup strategy for etcd (if needed)
- [ ] Migration scripts for existing file data (if applicable)

## Step-by-Step Migration

### Step 1: Identify Data Structures

**Action:** Map file-based data structures to CRD resources.

**Current State:**
```go
// FileStorage stores arbitrary []byte data
storage.Write(jsonData)
data, err := storage.Read()
```

**Target State:**
```go
// CRD resources with typed structures
type DeviceStorage struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              DeviceStorageSpec   `json:"spec,omitempty"`
    Status            DeviceStorageStatus `json:"status,omitempty"`
}
```

**Tasks:**
1. Review current file contents to identify data structures
2. Design CRD schema matching current data
3. Create CRD YAML definitions
4. Apply CRDs to cluster

### Step 2: Replace FileStorage Interface

**Action:** Create CRD-based storage interface to replace FileStorage.

**Current Interface:**
```go
type FileStorage interface {
    Read() ([]byte, error)
    Write(data []byte) error
    Exists() bool
    GetStorageInfo() StorageInfo
    ListBackups() ([]string, error)
}
```

**Target Interface:**
```go
type CRDStorage interface {
    Get(ctx context.Context, name string) (*DeviceStorage, error)
    Create(ctx context.Context, obj *DeviceStorage) error
    Update(ctx context.Context, obj *DeviceStorage) error
    Delete(ctx context.Context, name string) error
    List(ctx context.Context) (*DeviceStorageList, error)
}
```

**Implementation:**
```go
type crdStorage struct {
    client client.Client
}

func NewCRDStorage(c client.Client) CRDStorage {
    return &crdStorage{client: c}
}

func (s *crdStorage) Get(ctx context.Context, name string) (*DeviceStorage, error) {
    obj := &DeviceStorage{}
    err := s.client.Get(ctx, types.NamespacedName{Name: name}, obj)
    return obj, err
}

func (s *crdStorage) Create(ctx context.Context, obj *DeviceStorage) error {
    return s.client.Create(ctx, obj)
}

func (s *crdStorage) Update(ctx context.Context, obj *DeviceStorage) error {
    return s.client.Update(ctx, obj)
}
```

### Step 3: Remove Backup Management

**Action:** Remove application-level backup logic; rely on Kubernetes versioning.

**Current Code to Remove:**
- `createBackup()` method
- `cleanupOldBackups()` method
- `ListBackups()` method
- Backup file naming logic
- Backup rotation logic

**Replacement:**
- Use Kubernetes resource versioning for history
- Use etcd backups for disaster recovery
- Use Kubernetes backup tools (Velero) if needed

**Migration Notes:**
- No direct replacement needed - Kubernetes handles this
- If backup history is critical, use resource version tracking
- Consider implementing a watch-based history if needed

### Step 4: Replace Atomic Write Operations

**Action:** Replace temporary file pattern with Kubernetes API atomicity.

**Current Code:**
```go
func (fs *FileStorage) Write(data []byte) error {
    // 1. Create backup
    if err := fs.createBackup(); err != nil {
        return err
    }
    
    // 2. Write to temp file
    tmpPath := fs.filePath + ".tmp"
    if err := ioutil.WriteFile(tmpPath, data, 0644); err != nil {
        return err
    }
    
    // 3. Atomic move
    if err := os.Rename(tmpPath, fs.filePath); err != nil {
        os.Remove(tmpPath)
        return err
    }
    
    // 4. Cleanup old backups
    return fs.cleanupOldBackups()
}
```

**Target Code:**
```go
func (s *crdStorage) Update(ctx context.Context, obj *DeviceStorage) error {
    // Kubernetes API provides atomicity automatically
    // Use resource version for optimistic concurrency
    return s.client.Update(ctx, obj)
}

// With conflict handling:
func (s *crdStorage) UpdateWithRetry(ctx context.Context, obj *DeviceStorage) error {
    for {
        current := &DeviceStorage{}
        if err := s.client.Get(ctx, types.NamespacedName{Name: obj.Name}, current); err != nil {
            return err
        }
        
        // Update spec
        obj.Spec = desiredSpec
        obj.ResourceVersion = current.ResourceVersion
        
        if err := s.client.Update(ctx, obj); err != nil {
            if apierrors.IsConflict(err) {
                continue // Retry on conflict
            }
            return err
        }
        return nil
    }
}
```

### Step 5: Remove Recovery Logic

**Action:** Remove backup recovery; use Kubernetes resource recovery.

**Current Code to Remove:**
- `recoverFromBackup()` method
- Backup file discovery logic
- Recovery from backup on read failure

**Replacement:**
- Kubernetes API handles resource availability
- etcd provides automatic recovery
- Use resource version for conflict resolution
- Implement retry logic for transient failures

**Error Handling:**
```go
func (s *crdStorage) Get(ctx context.Context, name string) (*DeviceStorage, error) {
    obj := &DeviceStorage{}
    err := s.client.Get(ctx, types.NamespacedName{Name: name}, obj)
    
    if apierrors.IsNotFound(err) {
        // Resource doesn't exist - handle as needed
        return nil, err
    }
    
    // Transient errors - retry with backoff
    if apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) {
        return s.GetWithRetry(ctx, name)
    }
    
    return obj, err
}
```

### Step 6: Remove Thread Safety Mechanisms

**Action:** Remove mutex-based locking; use Kubernetes resource version.

**Current Code to Remove:**
```go
type FileStorage struct {
    mu        sync.RWMutex
    filePath  string
    backupCount int
}

func (fs *FileStorage) Read() ([]byte, error) {
    fs.mu.RLock()
    defer fs.mu.RUnlock()
    // ...
}
```

**Target Code:**
```go
type crdStorage struct {
    client client.Client
    // No mutex needed - Kubernetes handles concurrency
}

// Concurrency handled via resource version
func (s *crdStorage) Update(ctx context.Context, obj *DeviceStorage) error {
    // Resource version ensures atomic updates
    return s.client.Update(ctx, obj)
}
```

### Step 7: Replace Storage Information

**Action:** Replace file metadata with Kubernetes resource status.

**Current Code:**
```go
type StorageInfo struct {
    FilePath      string
    BackupCount   int
    Exists        bool
    FileSize      int64
    ModifiedTime  time.Time
    Backups       []string
}
```

**Target Code:**
```go
type DeviceStorageStatus struct {
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    LastSync   metav1.Time        `json:"lastSync,omitempty"`
    // Add status fields as needed
}

// Use CRD status for state information
func (s *crdStorage) GetStatus(ctx context.Context, name string) (*DeviceStorageStatus, error) {
    obj, err := s.Get(ctx, name)
    if err != nil {
        return nil, err
    }
    return &obj.Status, nil
}
```

### Step 8: Update Configuration

**Action:** Remove file storage configuration; add Kubernetes client configuration.

**Current Configuration:**
```yaml
device:
  storage:
    path: "/var/lib/dns-operator/storage.json"
    backup_count: 3
```

**Target Configuration:**
```yaml
# No storage configuration needed - uses Kubernetes defaults
# Or configure namespace:
kubernetes:
  namespace: "dns-operator"
```

**Code Changes:**
```go
// Remove:
type StorageConfig struct {
    Path       string `yaml:"path"`
    BackupCount int   `yaml:"backup_count"`
}

// Add:
type KubernetesConfig struct {
    Namespace string `yaml:"namespace"`
}
```

### Step 9: Data Migration (If Applicable)

**Action:** Migrate existing file data to CRD resources.

**Migration Script Pattern:**
```go
func MigrateFileToCRD(filePath string, client client.Client) error {
    // 1. Read existing file
    data, err := ioutil.ReadFile(filePath)
    if err != nil {
        return err
    }
    
    // 2. Parse JSON data
    var fileData YourDataStructure
    if err := json.Unmarshal(data, &fileData); err != nil {
        return err
    }
    
    // 3. Create CRD resource
    crdObj := &DeviceStorage{
        ObjectMeta: metav1.ObjectMeta{
            Name: "migrated-storage",
        },
        Spec: DeviceStorageSpec{
            // Map fileData to Spec
        },
    }
    
    // 4. Create in Kubernetes
    return client.Create(context.Background(), crdObj)
}
```

**Migration Checklist:**
- [ ] Backup existing file data
- [ ] Create migration script
- [ ] Test migration on non-production data
- [ ] Run migration during maintenance window
- [ ] Verify migrated data
- [ ] Remove old file storage code

### Step 10: Update Consumers

**Action:** Update all code that uses FileStorage to use CRDStorage.

**Find All Usages:**
```bash
grep -r "FileStorage" --include="*.go"
grep -r "NewFileStorage" --include="*.go"
grep -r "storage.Read" --include="*.go"
grep -r "storage.Write" --include="*.go"
```

**Update Pattern:**
```go
// Before:
storage := persistence.NewFileStorage(path, backupCount)
data, err := storage.Read()
err = storage.Write(newData)

// After:
storage := persistence.NewCRDStorage(k8sClient)
obj, err := storage.Get(ctx, "storage-name")
obj.Spec.Data = newData
err = storage.Update(ctx, obj)
```

## Testing Strategy

### Unit Tests

**Remove:**
- File operation tests
- Backup creation tests
- Recovery tests
- Atomic write tests

**Add:**
- CRD creation/update tests
- Resource version conflict tests
- Error handling tests
- Status update tests

**Example Test:**
```go
func TestCRDStorage_Update(t *testing.T) {
    scheme := runtime.NewScheme()
    _ = AddToScheme(scheme)
    
    fakeClient := fake.NewClientBuilder().
        WithScheme(scheme).
        Build()
    
    storage := NewCRDStorage(fakeClient)
    
    obj := &DeviceStorage{
        ObjectMeta: metav1.ObjectMeta{Name: "test"},
        Spec: DeviceStorageSpec{Data: "test"},
    }
    
    err := storage.Create(context.Background(), obj)
    assert.NoError(t, err)
    
    obj.Spec.Data = "updated"
    err = storage.Update(context.Background(), obj)
    assert.NoError(t, err)
}
```

### Integration Tests

- Test CRD operations against test cluster
- Test resource version conflicts
- Test error scenarios (not found, conflicts)
- Test concurrent updates

## Rollback Plan

If migration issues occur:

1. **Keep FileStorage code** - Don't delete immediately
2. **Feature flag** - Use feature flag to switch between implementations
3. **Dual write** - Write to both during transition
4. **Data export** - Export CRD data back to files if needed

**Rollback Implementation:**
```go
type StorageAdapter interface {
    Read() ([]byte, error)
    Write(data []byte) error
}

// Implement both FileStorage and CRDStorage as adapters
// Switch via configuration
```

## Post-Migration Validation

### Verification Checklist

- [ ] All FileStorage usages replaced
- [ ] CRD resources created successfully
- [ ] Data migrated correctly (if applicable)
- [ ] No file storage code remaining
- [ ] Configuration updated
- [ ] Tests passing
- [ ] Performance acceptable
- [ ] Monitoring/observability updated

### Performance Considerations

**File Storage:**
- Local file I/O (fast)
- No network overhead
- Limited scalability

**CRD Storage:**
- Network calls to API server
- etcd latency
- Better scalability
- Consider caching if needed

**Optimization:**
- Use informers for read-heavy workloads
- Cache frequently accessed resources
- Batch operations when possible

## Benefits of Migration

### Automatic Persistence
- CRDs stored in etcd automatically
- No manual persistence logic needed
- Distributed and highly available

### Versioning
- Kubernetes resource versioning
- History via etcd
- No manual version management

### Backup and Recovery
- etcd backups for disaster recovery
- Kubernetes backup tools (Velero)
- No application-level backup needed

### Concurrency Control
- Resource version for optimistic concurrency
- Kubernetes API handles conflicts
- No manual locking needed

### Observability
- Kubernetes-native metrics
- Resource events
- Better debugging capabilities

## Common Pitfalls

1. **Resource Version Conflicts**
   - Always read before update
   - Handle conflict errors gracefully
   - Implement retry logic

2. **Namespace Management**
   - Ensure correct namespace is used
   - Handle namespace creation if needed

3. **CRD Schema Changes**
   - Plan for backward compatibility
   - Use conversion webhooks if needed

4. **Performance**
   - Monitor API server load
   - Use informers for read-heavy workloads
   - Consider rate limiting

## References

- [Kubernetes CRD Documentation](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/)
- [Controller-Runtime Client](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client)
- [Resource Version and Concurrency](https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions)
- Original research: `docs/research/persistence.md`

