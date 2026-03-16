# Research Documentation Summary

## Overview

The `docs/research/` directory contains comprehensive reference architecture documentation for each domain in the DNS operator project. Each document analyzes the current reference implementation and provides detailed migration considerations for transitioning to a Kubernetes operator pattern. All documents follow a consistent structure covering architecture patterns, core components, data flows, and migration strategies.

## Core Domains

### 1. **API Domain** (`api.md`)
- **Purpose**: REST API layer with handler registry pattern
- **Key Features**: Route registration via `RouteInfo` structs, modular handler initialization, dependency injection
- **Migration**: Replace HTTP handlers with Kubernetes controllers; use Kubernetes API server for CRD management

### 2. **Certificate Domain** (`certificate.md`)
- **Purpose**: SSL/TLS certificate lifecycle management with Let's Encrypt
- **Key Features**: ACME DNS-01 challenges via Cloudflare, SAN management, automatic renewal with backoff
- **Migration**: Certificate CRD; store certificates in Kubernetes Secrets; controller-based renewal

### 3. **DNS Domain** (`dns.md`)
- **Purpose**: DNS record and zone file management with CoreDNS integration
- **Key Features**: Zone file-based DNS, template-based Corefile generation, record validation
- **Migration**: DNSRecord CRD; zone files in ConfigMaps; controller aggregates records by zone

### 4. **Proxy Domain** (`proxy.md`)
- **Purpose**: Caddy reverse proxy configuration management
- **Key Features**: Template-based Caddyfile generation, rule persistence, automatic reload
- **Migration**: ProxyRule CRD; Caddyfile in ConfigMap; Caddy as separate Deployment

### 5. **Tailscale Domain** (`tailscale.md`)
- **Purpose**: Tailscale API integration for device discovery and IP resolution
- **Key Features**: Device synchronization, IP resolution (100.64.x.x range), polling-based sync
- **Migration**: TailscaleDevice CRD; controller polling; DNSRecord references TailscaleDevice for IP resolution

## Infrastructure Domains

### 6. **Configuration** (`config.md`)
- **Purpose**: Centralized configuration management using Viper
- **Key Features**: YAML files with environment variable overrides, required key validation, hot-reload capability
- **Migration**: ConfigMaps for non-sensitive data; Secrets for sensitive data; CRD specs for resource config

### 7. **Logging** (`logging.md`)
- **Purpose**: Centralized structured logging with Zap
- **Key Features**: Singleton pattern, configurable log levels, thread-safe operations
- **Migration**: Continue with Zap; add Kubernetes context; JSON logging for production

### 8. **Persistence** (`persistence.md`)
- **Purpose**: File-based storage with atomic operations and backup management
- **Key Features**: Thread-safe file operations, automatic backup creation, recovery from corruption
- **Migration**: Replace with CRD storage in etcd; remove file storage layer

### 9. **Healthcheck** (`healthcheck.md`)
- **Purpose**: Health checking capabilities with aggregation support
- **Key Features**: Interface-based design, component-specific checkers, latency measurement
- **Migration**: Kubernetes liveness/readiness probes; CRD status for resource health

### 10. **Firewall** (`firewall.md`)
- **Purpose**: Linux firewall rule management using ipset and iptables
- **Key Features**: Tailscale CIDR protection (100.64.0.0/10), automatic rule creation/cleanup
- **Migration**: Kubernetes NetworkPolicy resources; remove privileged container requirements

## Command Domains

### 11. **API Server Command** (`cmd-api.md`)
- **Purpose**: Main entry point for DNS Manager service
- **Key Features**: Component initialization, HTTP/HTTPS server management, background processes
- **Migration**: Replace with controller-runtime manager; remove HTTP server

### 12. **OpenAPI Generation** (`cmd-generate-openapi.md`)
- **Purpose**: Build-time OpenAPI specification generation via AST analysis
- **Key Features**: Route discovery, type introspection, OpenAPI 3.0 spec generation
- **Migration**: Use kubebuilder for CRD schemas; adapt generator for webhook schemas

## Supporting Domains

### 13. **Documentation** (`docs.md`)
- **Purpose**: Swagger UI and OpenAPI specification serving
- **Key Features**: Protocol detection, theme support, static file serving
- **Migration**: Remove HTTP docs; use Kubernetes-native documentation (`kubectl explain`)

### 14. **Validation** (`validation.md`)
- **Purpose**: RFC-compliant DNS and domain name validation utilities
- **Key Features**: FQDN validation, domain validation, public package (pkg/)
- **Migration**: Maintain package; use in validating webhooks; generate OpenAPI schema

## Common Migration Patterns

### 1. **CRD-Based State Management**
- Replace file-based storage with CRD storage in etcd
- CRDs become the source of truth
- Automatic persistence and versioning via Kubernetes

### 2. **Controller Reconciliation**
- Replace HTTP handlers and background processes with controllers
- Use controller-runtime's RequeueAfter for polling
- Watch CRDs and reconcile state

### 3. **ConfigMap/Secret Storage**
- Move configuration to Kubernetes-native resources
- ConfigMaps for non-sensitive data
- Secrets for sensitive data (API keys, tokens)

### 4. **Webhook Validation**
- Use validating/mutating webhooks for complex validation
- OpenAPI schema for basic format validation
- Webhooks for business logic validation

### 5. **Status Subresources**
- Use CRD status for health and state information
- Status conditions for component health
- Resource metadata for information

### 6. **Native Networking**
- Replace custom firewall with NetworkPolicy resources
- Use Kubernetes Service ports for policy definition
- No privileged container requirements

## Architecture Patterns Identified

1. **Manager-Based Patterns**: Certificate, Proxy, Firewall domains use manager objects for lifecycle management
2. **Service-Based Patterns**: DNS domain uses a service layer for business logic
3. **Handler Registry**: API domain uses centralized handler registration
4. **Singleton Patterns**: Configuration and Logging use singleton instances
5. **Client-Based Integration**: Tailscale domain uses API client pattern
6. **Utility Functions**: Validation domain provides reusable utility functions

## Document Structure

All research documents follow a consistent structure:

1. **Executive Summary**: High-level overview of the domain
2. **Architecture Overview**: Current architecture pattern with diagrams
3. **Core Components**: Detailed component breakdown
4. **Data Flow**: Request/response and operation flows
5. **CRD Mapping Considerations**: Proposed CRD structures and reconciliation logic
6. **Key Migration Considerations**: Step-by-step migration guidance
7. **Testing Strategy**: Current and target testing approaches
8. **Summary**: Key takeaways and migration highlights

## Key Takeaways

- **Kubernetes-Native Approach**: All domains migrate to Kubernetes-native patterns (CRDs, controllers, ConfigMaps, Secrets)
- **Separation of Concerns**: Clear separation between resource management (CRDs) and infrastructure (controllers)
- **Declarative Configuration**: Move from imperative HTTP APIs to declarative CRD-based configuration
- **State Management**: etcd replaces file-based storage for all state
- **Validation**: Multi-layer validation (OpenAPI schema + webhooks + controller validation)
- **Observability**: Kubernetes probes and CRD status replace custom health checks

## Next Steps

1. Review individual domain documents for detailed migration plans
2. Design CRD schemas based on proposed structures
3. Implement controllers using controller-runtime
4. Set up webhook infrastructure for validation
5. Migrate configuration to ConfigMaps and Secrets
6. Replace file storage with CRD-based state management

