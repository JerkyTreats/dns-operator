# Persistence Domain Reference Architecture

## Executive Summary

The Persistence domain (`reference/internal/persistence/`) provides file-based storage with atomic operations, backup management, and recovery capabilities. It implements thread-safe file operations with automatic backup creation and recovery from corruption.

## Architecture Overview

### Current Architecture Pattern

The Persistence domain follows a **file storage pattern with backup management**:

```
┌─────────────────────────────────────────────────────────────┐
│                    FileStorage                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Atomic Write  │  │ Backup Mgr   │  │ Recovery     │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Thread-safe file operations
- Atomic writes via temporary files
- Automatic backup creation
- Backup rotation and cleanup
- Recovery from backup on corruption

## Core Components

### 1. File Storage

**Location:** `internal/persistence/file.go`

**Responsibilities:**
- File read/write operations
- Atomic write operations
- Thread-safe access
- Backup management
- Recovery operations

**Key Operations:**
```go
NewFileStorage() *FileStorage
NewFileStorageWithPath(filePath string, backupCount int) *FileStorage
Read() ([]byte, error)
Write(data []byte) error
Exists() bool
```

**Configuration:**
- File path from config: `device.storage.path`
- Backup count from config: `device.storage.backup_count`
- Default backup count: 3

### 2. Atomic Write Operations

**Location:** `internal/persistence/file.go` (Write)

**Responsibilities:**
- Write to temporary file first
- Atomic move to target location
- Error handling and cleanup
- Directory creation

**Write Flow:**
1. Create backup (if file exists)
2. Write to temporary file (`{filepath}.tmp`)
3. Atomically move temp file to target
4. Clean up on failure

### 3. Backup Management

**Location:** `internal/persistence/file.go` (createBackup, cleanupOldBackups)

**Responsibilities:**
- Automatic backup creation before writes
- Timestamp-based backup naming
- Backup rotation (keep N backups)
- Old backup cleanup

**Backup Naming:**
- Format: `{filepath}.backup.{timestamp}`
- Timestamp: `20060102-150405` (YYYYMMDD-HHMMSS)
- Sorted by modification time

**Key Operations:**
```go
createBackup() error
cleanupOldBackups() error
ListBackups() ([]string, error)
```

### 4. Recovery

**Location:** `internal/persistence/file.go` (recoverFromBackup)

**Responsibilities:**
- Automatic recovery from backup on read failure
- Most recent backup selection
- Backup file discovery
- Recovery logging

**Recovery Flow:**
1. Read failure detected
2. Discover backup files
3. Sort by modification time (newest first)
4. Read from most recent backup
5. Return recovered data

### 5. Storage Information

**Location:** `internal/persistence/file.go` (GetStorageInfo)

**Responsibilities:**
- Storage metadata
- File information
- Backup listing
- Storage statistics

**Information Provided:**
- File path
- Backup count configuration
- File existence
- File size and modification time
- Available backups

## Data Flow

### Current Flow: File Write

```
1. Write(data)
   ↓
2. Create backup (if file exists)
   ├─→ Read current file
   └─→ Write to backup file
   ↓
3. Write to temporary file
   ├─→ Write data to {filepath}.tmp
   └─→ Handle errors
   ↓
4. Atomic move
   ├─→ Rename temp file to target
   └─→ Clean up on failure
   ↓
5. Cleanup old backups
   └─→ Remove backups beyond limit
```

### Current Flow: File Read

```
1. Read()
   ↓
2. Read file
   ├─→ Success → return data
   └─→ Failure → recover from backup
   ↓
3. Recovery (if needed)
   ├─→ Discover backup files
   ├─→ Sort by modification time
   └─→ Read from most recent backup
```

## CRD Mapping Considerations

### Kubernetes-Native Persistence

**Option 1: CRD Storage (etcd)**
- CRDs stored in etcd (Kubernetes-native)
- No file-based persistence needed
- Automatic persistence and versioning
- Distributed and highly available

**Option 2: ConfigMap/Secret Storage**
- Store data in ConfigMaps or Secrets
- Kubernetes-native storage
- Version history via Kubernetes
- No backup/restore needed

**Option 3: Persistent Volumes**
- Use PersistentVolumes for file storage
- Maintain file-based approach
- Kubernetes-managed storage
- Backup via volume snapshots

**Recommended Approach:**
- **Option 1** - Use CRD storage (etcd)
- CRDs are the source of truth
- No file-based persistence needed
- Leverage Kubernetes-native storage
- Remove file storage layer

## Key Migration Considerations

### 1. State Storage

**Current:** File-based JSON storage
**Target:** CRD storage in etcd

**Migration:**
- Replace file storage with CRD resources
- CRDs stored in etcd automatically
- No manual backup/restore needed
- Use CRD status for computed state

### 2. Backup Management

**Current:** Automatic backup creation and rotation
**Target:** Kubernetes versioning and etcd backups

**Migration:**
- Remove backup management logic
- Use Kubernetes resource versioning
- Rely on etcd backups for disaster recovery
- Use Kubernetes backup tools (velero, etc.)

### 3. Atomic Operations

**Current:** Atomic file writes via temporary files
**Target:** Kubernetes API atomicity

**Migration:**
- Kubernetes API provides atomicity
- CRD updates are atomic
- No need for temporary file pattern
- Use optimistic concurrency (resource version)

### 4. Recovery

**Current:** Recovery from backup files
**Target:** Kubernetes resource recovery

**Migration:**
- Remove backup recovery logic
- Use Kubernetes resource recovery tools
- etcd provides automatic recovery
- Use resource version for conflict resolution

### 5. Thread Safety

**Current:** Mutex-based thread safety
**Target:** Kubernetes API concurrency control

**Migration:**
- Remove mutex-based synchronization
- Use Kubernetes resource version for concurrency
- Controller-runtime handles concurrency
- No manual locking needed

### 6. Storage Information

**Current:** File metadata and backup listing
**Target:** Kubernetes resource status

**Migration:**
- Replace storage info with CRD status
- Use status conditions for state
- Resource metadata for information
- Remove file-based metadata

## Kubernetes Storage Benefits

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
- Kubernetes backup tools (velero)
- No application-level backup needed

### Concurrency Control
- Resource version for optimistic concurrency
- Kubernetes API handles conflicts
- No manual locking needed

## Testing Strategy

### Current Testing Approach
- File operation tests
- Backup creation tests
- Recovery tests
- Atomic write tests

### Target Testing Approach
- CRD creation/update tests
- Resource version tests
- Conflict resolution tests
- Status update tests

## Summary

The Persistence domain provides file-based storage with atomic operations and backup management. Migration to Kubernetes will:

1. **CRD Storage** - Use etcd for CRD storage (Kubernetes-native)
2. **Remove File Storage** - No file-based persistence needed
3. **Remove Backup Logic** - Rely on etcd backups and Kubernetes tools
4. **Remove Atomic Write Logic** - Kubernetes API provides atomicity
5. **Remove Recovery Logic** - Use Kubernetes resource recovery
6. **Remove Thread Safety** - Kubernetes API handles concurrency

The domain's file storage functionality is completely replaced by Kubernetes-native CRD storage in etcd, providing automatic persistence, versioning, and concurrency control without application-level management.


