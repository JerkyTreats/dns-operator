# Configuration Domain Reference Architecture

## Executive Summary

The Configuration domain from the legacy reference repo under `internal/config/` provides centralized configuration management using spf13/viper. It supports YAML configuration files, environment variable overrides, required key validation, and hot-reload capabilities. The domain follows a singleton pattern with thread-safe access.

## Architecture Overview

### Current Architecture Pattern

The Configuration domain follows a **singleton configuration pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Config Singleton                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Viper        │  │ Required Keys│  │ Search Paths  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │ Env Override │  │ Hot Reload    │                        │
│  └──────────────┘  └──────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Singleton pattern with thread-safe access
- Viper-based configuration loading
- YAML file support with search paths
- Environment variable overrides
- Required key validation
- Optional hot-reload capability

## Core Components

### 1. Config Singleton

**Location:** `internal/config/config.go`

**Responsibilities:**
- Centralized configuration access
- Thread-safe read/write operations
- Lazy initialization
- Configuration loading and validation

**Key Operations:**
```go
InitConfig(opts ...ConfigOption) error
GetString(key string) string
GetInt(key string) int
GetBool(key string) bool
GetDuration(key string) time.Duration
Reload() error
```

**Initialization:**
- Supports explicit config file path
- Supports search paths for config discovery
- Automatic environment variable binding
- Default value management

### 2. Configuration Options

**Location:** `internal/config/config.go`

**Responsibilities:**
- Functional options pattern for configuration
- Config file path specification
- Search path management

**Key Options:**
```go
WithConfigPath(path string) ConfigOption
WithSearchPaths(paths ...string) ConfigOption
WithOnlySearchPaths(paths ...string) ConfigOption
```

**Usage:**
- Explicit config file takes precedence over search paths
- Search paths used when no explicit path provided
- Supports multiple search paths

### 3. Required Key Validation

**Location:** `internal/config/config.go`

**Responsibilities:**
- Required configuration key registration
- Validation on startup
- Missing key reporting

**Key Operations:**
```go
RegisterRequiredKey(key string)
CheckRequiredKeys() error
```

**Validation Flow:**
1. Modules register required keys during init
2. `CheckRequiredKeys()` validates all registered keys
3. Reports missing keys with error message
4. Application exits if required keys missing

### 4. Configuration Keys

**Location:** `internal/config/config.go` (constants)

**Responsibilities:**
- Centralized key definitions
- Key naming conventions
- Documentation of configuration structure

**Key Categories:**
- Server configuration (host, port, TLS)
- DNS configuration (domain, zones, CoreDNS)
- Tailscale configuration (API key, tailnet)
- Certificate configuration (email, provider)
- Proxy configuration (enabled, paths)
- Logging configuration (level)

### 5. Sync Configuration

**Location:** `internal/config/config.go`

**Responsibilities:**
- Structured configuration for DNS sync
- Polling configuration
- Configuration unmarshaling

**Key Operations:**
```go
GetSyncConfig() SyncConfig
```

**Structure:**
```go
type SyncConfig struct {
    Enabled bool
    Origin  string
    Polling PollingConfig
}

type PollingConfig struct {
    Enabled  bool
    Interval time.Duration
}
```

## Data Flow

### Current Flow: Configuration Loading

```
1. Application Startup
   ↓
2. InitConfig() called
   ├─→ Load config file (YAML)
   ├─→ Apply environment variable overrides
   ├─→ Set default values
   └─→ Initialize Viper instance
   ↓
3. Modules register required keys
   ↓
4. CheckRequiredKeys() validates
   ↓
5. Configuration ready for use
```

### Current Flow: Configuration Access

```
1. GetString(key) / GetInt(key) / GetBool(key)
   ↓
2. Ensure config initialized (lazy loading)
   ↓
3. Thread-safe read lock
   ↓
4. Viper.GetString(key) / GetInt(key) / GetBool(key)
   ↓
5. Return value (or default)
```

## CRD Mapping Considerations

### Kubernetes-Native Configuration

**Option 1: ConfigMaps for Non-Sensitive Data**
- Store configuration in ConfigMaps
- Mount ConfigMaps as volumes in pods
- Support hot-reload via ConfigMap watching

**Option 2: Secrets for Sensitive Data**
- Store API keys, tokens in Secrets
- Mount Secrets as volumes or use Secret references
- RBAC-controlled access

**Option 3: CRD Spec for Resource-Specific Config**
- Embed configuration in CRD specs
- Resource-level configuration
- Declarative configuration management

**Option 4: Operator-Level ConfigMap**
- Global operator configuration in ConfigMap
- Operator-specific settings
- Default values for resources

### Recommended Approach

**Hybrid Configuration Strategy:**
1. **ConfigMaps** for non-sensitive operator configuration
2. **Secrets** for sensitive data (API keys, tokens)
3. **CRD Specs** for resource-specific configuration
4. **Environment Variables** for deployment-specific overrides

## Key Migration Considerations

### 1. Configuration Structure

**Current:** Single YAML file with nested keys
**Target:** Kubernetes ConfigMaps and Secrets

**Migration:**
- Convert YAML config to ConfigMap
- Extract secrets to Kubernetes Secrets
- Maintain configuration structure in ConfigMap data
- Support environment variable overrides

### 2. Required Key Validation

**Current:** Runtime validation of required keys
**Target:** CRD validation and webhook validation

**Migration:**
- Move validation to CRD OpenAPI schema
- Use validating webhooks for complex validation
- Operator startup validation for global config
- Remove runtime required key checks

### 3. Configuration Access

**Current:** Singleton pattern with GetString/GetInt/GetBool
**Target:** Kubernetes client for ConfigMap/Secret access

**Migration:**
- Use controller-runtime client for ConfigMap access
- Cache configuration in controller
- Watch ConfigMaps for changes
- Update cached configuration on changes

### 4. Hot Reload

**Current:** Reload() method for configuration refresh
**Target:** ConfigMap watching and reconciliation

**Migration:**
- Watch ConfigMaps for changes
- Trigger reconciliation on ConfigMap updates
- Update controller state from new configuration
- Maintain backward compatibility during transition

### 5. Search Paths

**Current:** Multiple search paths for config discovery
**Target:** Explicit ConfigMap references

**Migration:**
- Remove search path logic
- Use explicit ConfigMap references
- Support multiple ConfigMaps for different config sections
- Maintain precedence order

### 6. Environment Variable Overrides

**Current:** Automatic environment variable binding via Viper
**Target:** Environment variables in Deployment/StatefulSet

**Migration:**
- Support environment variables in pod spec
- Maintain environment variable precedence
- Use ConfigMap/Secret references in env vars
- Support both approaches during transition

## Testing Strategy

### Current Testing Approach
- Unit tests with test configuration
- SetForTest() for test overrides
- ResetForTest() for test cleanup
- Configuration validation tests

### Target Testing Approach
- Controller tests with fake ConfigMap/Secret client
- Configuration loading tests
- ConfigMap watching tests
- Validation tests with webhooks

## Summary

The Configuration domain provides centralized configuration management with Viper. Migration to Kubernetes will:

1. **ConfigMaps** - Non-sensitive configuration in ConfigMaps
2. **Secrets** - Sensitive data in Kubernetes Secrets
3. **CRD Specs** - Resource-specific configuration in CRD specs
4. **Controller Access** - Use controller-runtime client for configuration access
5. **ConfigMap Watching** - Watch ConfigMaps for configuration changes
6. **Validation** - Move validation to CRD schema and webhooks

The singleton pattern and required key validation can be adapted for operator-level configuration management, while resource-specific configuration moves to CRD specs.

