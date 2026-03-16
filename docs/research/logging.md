# Logging Domain Reference Architecture

## Executive Summary

The Logging domain (`reference/internal/logging/`) provides centralized logging using uber-go/zap. It implements a singleton pattern with configurable log levels, structured logging, and thread-safe access. All application code must use this package for logging.

## Architecture Overview

### Current Architecture Pattern

The Logging domain follows a **singleton logger pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Logging Singleton                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Zap Logger   │  │ Log Level     │  │ Config       │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Singleton pattern with lazy initialization
- Zap-based structured logging
- Configurable log levels (DEBUG, INFO, WARN, ERROR, NONE)
- Thread-safe logging operations
- Development configuration with caller information

## Core Components

### 1. Logger Singleton

**Location:** `internal/logging/logging.go`

**Responsibilities:**
- Centralized logger instance management
- Lazy initialization
- Thread-safe access
- Log level configuration

**Key Operations:**
```go
Info(format string, args ...interface{})
Debug(format string, args ...interface{})
Warn(format string, args ...interface{})
Error(format string, args ...interface{})
Sync() error
```

**Initialization:**
- Lazy initialization on first log call
- Zap development configuration
- Caller information included
- Log level from configuration

### 2. Log Level Management

**Location:** `internal/logging/logging.go` (getZapLevel)

**Responsibilities:**
- Log level mapping from config to zap
- Support for NONE level (silence all logs)
- Default to INFO level

**Supported Levels:**
- `DEBUG` - Debug level logging
- `INFO` - Info level logging (default)
- `WARN` - Warning level logging
- `ERROR` - Error level logging
- `NONE` - Silence all logs (for testing)

**Configuration:**
- Log level from config key: `logging.level`
- Environment variable support via config
- Default: INFO level

### 3. Zap Integration

**Location:** `internal/logging/logging.go` (initLogger)

**Responsibilities:**
- Zap logger initialization
- Development configuration
- Caller skip configuration
- Sugared logger for convenience

**Configuration:**
- Development config (human-readable)
- Caller information included
- Caller skip: 1 (skip logging package itself)
- Sugared logger for formatted logging

## Data Flow

### Current Flow: Logging Initialization

```
1. First log call (Info/Debug/Warn/Error)
   ↓
2. initLogger() called (lazy initialization)
   ├─→ Get log level from config
   ├─→ Create zap development config
   ├─→ Set log level
   ├─→ Add caller information
   └─→ Build sugared logger
   ↓
3. Logger ready for use
```

### Current Flow: Log Message

```
1. Info("message %s", arg)
   ↓
2. Ensure logger initialized
   ↓
3. logger.Infof("message %s", arg)
   ↓
4. Zap processes log
   ├─→ Format message
   ├─→ Add caller information
   └─→ Output to stderr
```

## CRD Mapping Considerations

### Kubernetes-Native Logging

**Option 1: Structured Logging**
- Continue using zap for structured logging
- Kubernetes-native log format
- JSON logging for production

**Option 2: Kubernetes Logging Standards**
- Follow Kubernetes logging conventions
- Use klog for consistency
- Structured logging with context

**Option 3: Operator SDK Logging**
- Use Operator SDK logging utilities
- Consistent with Kubernetes operators
- Structured logging support

**Recommended Approach:**
- **Option 1** - Continue using zap with Kubernetes conventions
- Use structured logging (JSON in production)
- Add Kubernetes context (namespace, resource, etc.)
- Maintain log level configuration
- Use controller-runtime logging if needed

## Key Migration Considerations

### 1. Logging Library

**Current:** Zap for structured logging
**Target:** Continue with zap or use controller-runtime logging

**Migration:**
- Continue using zap (recommended)
- Or migrate to controller-runtime logging
- Maintain structured logging format
- Add Kubernetes context fields

### 2. Log Level Configuration

**Current:** Config-based log level
**Target:** Environment variable or ConfigMap

**Migration:**
- Use environment variable for log level
- Or ConfigMap for operator configuration
- Maintain same log level values
- Support dynamic log level changes if needed

### 3. Structured Logging

**Current:** Development config (human-readable)
**Target:** JSON logging for production

**Migration:**
- Use JSON encoding for production
- Development config for local development
- Add Kubernetes context fields
- Maintain caller information

### 4. Context Logging

**Current:** Simple formatted logging
**Target:** Context-aware logging with Kubernetes metadata

**Migration:**
- Add namespace, resource name, etc. to logs
- Use structured fields for context
- Include reconciliation context
- Add request/response logging for webhooks

### 5. Log Output

**Current:** stderr output
**Target:** Kubernetes log aggregation

**Migration:**
- Continue using stderr (Kubernetes standard)
- Logs collected by Kubernetes
- Support log aggregation tools
- Maintain log format consistency

## Kubernetes Logging Best Practices

### Structured Logging Example

```go
logger.Info("Reconciling DNSRecord",
    zap.String("namespace", req.Namespace),
    zap.String("name", req.Name),
    zap.String("fqdn", dnsRecord.Spec.FQDN),
)
```

### Log Level Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dns-operator-config
data:
  log-level: "INFO"  # DEBUG, INFO, WARN, ERROR
```

### Environment Variable

```yaml
env:
- name: LOG_LEVEL
  value: "INFO"
```

## Testing Strategy

### Current Testing Approach
- Reset logger for tests
- NONE log level for test silence
- Mock logger if needed

### Target Testing Approach
- Continue with test logger reset
- Use test log level configuration
- Structured logging in tests
- Log assertion tests if needed

## Summary

The Logging domain provides centralized structured logging with zap. Migration to Kubernetes will:

1. **Continue with Zap** - Maintain zap for structured logging
2. **JSON Logging** - Use JSON encoding for production
3. **Kubernetes Context** - Add Kubernetes metadata to logs
4. **Environment Configuration** - Use environment variables or ConfigMap for log level
5. **Structured Fields** - Use structured fields for context information

The domain's singleton pattern and structured logging approach should be maintained, with additions for Kubernetes context and JSON encoding for production environments.


