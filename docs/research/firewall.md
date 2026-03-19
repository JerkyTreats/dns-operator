# Firewall Domain Reference Architecture

## Executive Summary

The Firewall domain from the legacy reference repo under `internal/firewall/` manages Linux firewall rules using ipset and iptables to allow Tailscale CIDR ranges such as `100.64.0.0/10` access to DNS and API services. It provides automatic firewall rule management with validation and cleanup capabilities.

THIS FEATURE WILL NOT BE MIGRATED 

## Architecture Overview

### Current Architecture Pattern

The Firewall domain follows a **manager-based rule management pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Firewall Manager                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Ipset        │  │ Iptables     │  │ Validation    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- ipset for CIDR range management
- iptables for firewall rules
- Automatic rule creation and cleanup
- Tailscale CIDR protection (100.64.0.0/10)
- Port-based rule management

## Core Components

### 1. Firewall Manager

**Location:** `internal/firewall/ipset.go`

**Responsibilities:**
- Firewall rule lifecycle management
- Ipset creation and management
- Iptables rule management
- Rule validation
- Cleanup operations

**Key Operations:**
```go
NewManager() (*Manager, error)
EnsureFirewallRules() error
RemoveFirewallRules() error
ValidateFirewallSetup() error
ListCurrentRules() ([]string, error)
```

**Configuration:**
- Tailscale CIDR: `100.64.0.0/10` (hardcoded)
- Ipset name: `tailscale_allowed` (hardcoded)
- Ports: DNS (53), HTTP server, HTTPS server (from config)

### 2. Ipset Management

**Location:** `internal/firewall/ipset.go` (ensureIpsetExists, addCIDRToIpset)

**Responsibilities:**
- Create ipset if it doesn't exist
- Add Tailscale CIDR to ipset
- Ipset type: `hash:net`

**Key Operations:**
```go
ensureIpsetExists() error
addCIDRToIpset() error
```

**Ipset Commands:**
- `ipset create tailscale_allowed hash:net`
- `ipset add tailscale_allowed 100.64.0.0/10 -exist`

### 3. Iptables Management

**Location:** `internal/firewall/ipset.go` (ensureIptablesRules, ensureIptablesRule)

**Responsibilities:**
- Create iptables rules for allowed ports
- Match ipset in iptables rules
- Support TCP and UDP protocols
- Port-specific rules

**Key Operations:**
```go
ensureIptablesRules() error
ensureIptablesRule(protocol, port string) error
removeIptablesRule(protocol, port string) error
```

**Iptables Rule Format:**
```
iptables -I INPUT -m set --match-set tailscale_allowed src -p {protocol} --dport {port} -j ACCEPT
```

### 4. Validation

**Location:** `internal/firewall/ipset.go` (ValidateFirewallSetup, validateIptablesRule)

**Responsibilities:**
- Validate ipset exists and contains CIDR
- Validate iptables rules exist
- Rule verification

**Key Operations:**
```go
ValidateFirewallSetup() error
validateIptablesRule(protocol, port string) error
```

## Data Flow

### Current Flow: Firewall Setup

```
1. EnsureFirewallRules()
   ↓
2. ensureIpsetExists()
   ├─→ Check if ipset exists
   └─→ Create ipset if needed
   ↓
3. addCIDRToIpset()
   ├─→ Add 100.64.0.0/10 to ipset
   └─→ Use -exist flag to avoid errors
   ↓
4. ensureIptablesRules()
   ├─→ For each protocol (TCP, UDP)
   ├─→ For each port (DNS, HTTP, HTTPS)
   └─→ Create iptables rule
   ↓
5. Rules configured
```

### Current Flow: Firewall Cleanup

```
1. RemoveFirewallRules()
   ↓
2. Remove iptables rules
   ├─→ For each protocol
   └─→ For each port
   ↓
3. Destroy ipset
   └─→ ipset destroy tailscale_allowed
```

## CRD Mapping Considerations

### Kubernetes-Native Firewall

**Option 1: Network Policies**
- Use Kubernetes NetworkPolicy resources
- Define policies for Tailscale CIDR access
- Pod-level network isolation

**Option 2: Service Mesh**
- Use service mesh (Istio, Linkerd) for network policies
- Traffic management and security
- More complex but feature-rich

**Option 3: Node-Level Firewall**
- Maintain ipset/iptables at node level
- DaemonSet for firewall management
- Node-level network policies

**Option 4: Remove Firewall Management**
- Rely on Kubernetes network isolation
- Use NetworkPolicy for pod-level rules
- Remove node-level firewall management

**Recommended Approach:**
- **Option 1** - Use Kubernetes NetworkPolicy resources
- Define NetworkPolicy for Tailscale CIDR access
- Remove node-level firewall management
- Use Kubernetes-native network policies

## Key Migration Considerations

### 1. Network Policy Migration

**Current:** ipset + iptables for firewall rules
**Target:** Kubernetes NetworkPolicy resources

**Migration:**
- Create NetworkPolicy CRDs for Tailscale CIDR access
- Define policies for DNS and API services
- Use NetworkPolicy controller for enforcement
- Remove ipset/iptables management

### 2. CIDR Management

**Current:** Hardcoded Tailscale CIDR (100.64.0.0/10)
**Target:** Configurable CIDR in NetworkPolicy

**Migration:**
- Make CIDR configurable in NetworkPolicy
- Support multiple CIDR ranges if needed
- Use NetworkPolicy spec for CIDR definition

### 3. Port Management

**Current:** Ports from server configuration
**Target:** Ports from Service definitions

**Migration:**
- Use Kubernetes Service ports
- Define NetworkPolicy based on Service ports
- Remove configuration-based port discovery

### 4. Privileged Access

**Current:** Requires privileged container with NET_ADMIN, NET_RAW
**Target:** No privileged access needed with NetworkPolicy

**Migration:**
- Remove privileged container requirements
- Use standard Kubernetes NetworkPolicy
- No special capabilities needed

### 5. Rule Validation

**Current:** Validation via ipset/iptables commands
**Target:** NetworkPolicy status validation

**Migration:**
- Use NetworkPolicy status for validation
- Check NetworkPolicy conditions
- Remove command-based validation

## NetworkPolicy Example

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-tailscale-cidr
  namespace: dns-operator
spec:
  podSelector:
    matchLabels:
      app: dns-operator
  policyTypes:
  - Ingress
  ingress:
  - from:
    - ipBlock:
        cidr: 100.64.0.0/10
    ports:
    - protocol: TCP
      port: 53
    - protocol: UDP
      port: 53
    - protocol: TCP
      port: 8080
    - protocol: TCP
      port: 8443
```

## Testing Strategy

### Current Testing Approach
- Unit tests for firewall operations
- Mock ipset/iptables commands
- Validation tests

### Target Testing Approach
- NetworkPolicy CRD tests
- NetworkPolicy validation tests
- Integration tests with testenv
- NetworkPolicy enforcement tests

## Summary

The Firewall domain manages Linux firewall rules using ipset and iptables. Migration to Kubernetes will:

1. **NetworkPolicy Resources** - Use Kubernetes NetworkPolicy instead of ipset/iptables
2. **No Privileged Access** - Remove need for privileged containers
3. **Service-Based Ports** - Use Kubernetes Service ports for policy definition
4. **Configurable CIDR** - Make CIDR configurable in NetworkPolicy
5. **Native Validation** - Use NetworkPolicy status for validation

The domain's firewall management logic should be replaced with NetworkPolicy resources, providing Kubernetes-native network security without requiring privileged access.

