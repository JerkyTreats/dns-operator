# Validation Domain Reference Architecture

## Executive Summary

The Validation domain (`reference/pkg/validation/`) provides validation utilities for DNS names, FQDNs, and domain names. It implements RFC-compliant validation with character checking and format validation. The package is in the `pkg/` directory, making it a public API that can be used by external consumers.

## Architecture Overview

### Current Architecture Pattern

The Validation domain follows a **utility function pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│                    Validation Package                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ FQDN         │  │ Domain       │  │ DNS Name     │      │
│  │ Validation   │  │ Validation   │  │ Validation   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics:**
- Public package (pkg/) for external use
- RFC-compliant validation
- Character and format checking
- Error messages with context
- Boolean and error-returning functions

## Core Components

### 1. FQDN Validation

**Location:** `pkg/validation/dns.go`

**Responsibilities:**
- Fully Qualified Domain Name validation
- Format checking
- Character validation
- Error reporting

**Key Operations:**
```go
ValidateFQDN(hostname string) error
IsValidFQDN(hostname string) bool
```

**Validation Rules:**
- Must contain at least one dot (domain separator)
- Cannot start or end with a dot
- Each label must start and end with alphanumeric
- Each label can contain alphanumeric and hyphens
- Labels cannot be empty

**Example:**
```go
err := validation.ValidateFQDN("app.internal.example.test")
// Returns nil if valid, error if invalid
```

### 2. Domain Validation

**Location:** `pkg/validation/domain.go`

**Responsibilities:**
- Domain name validation
- Domain format checking
- Character validation

**Key Operations:**
```go
ValidateDomain(domain string) error
IsValidDomain(domain string) bool
```

**Validation Rules:**
- Similar to FQDN validation
- Domain-specific rules
- Label validation

### 3. DNS Name Validation

**Location:** `pkg/validation/dns.go` (helper functions)

**Responsibilities:**
- Character validation
- Alphanumeric checking
- Hyphen validation

**Helper Functions:**
```go
isAlphanumeric(b byte) bool
isAlphanumericOrHyphen(r rune) bool
```

## Data Flow

### Current Flow: FQDN Validation

```
1. ValidateFQDN(hostname)
   ↓
2. Check empty
   ↓
3. Check for dot (domain separator)
   ↓
4. Check start/end with dot
   ↓
5. Split by dots and validate each label
   ├─→ Check label not empty
   ├─→ Check label starts/ends with alphanumeric
   └─→ Check label contains only valid characters
   ↓
6. Return error or nil
```

## CRD Mapping Considerations

### Kubernetes-Native Validation

**Option 1: CRD OpenAPI Schema Validation**
- Use OpenAPI schema for CRD validation
- Kubernetes-native validation
- Automatic validation on resource creation

**Option 2: Validating Webhooks**
- Use validating webhooks for complex validation
- Custom validation logic
- Runtime validation

**Option 3: Controller Validation**
- Validate in controller reconciliation
- Status updates for validation errors
- User-friendly error messages

**Recommended Approach:**
- **Option 1 + Option 2** - Combine OpenAPI schema with webhooks
- Use OpenAPI schema for basic format validation
- Use validating webhooks for complex business logic validation
- Maintain validation utilities for reuse

## Key Migration Considerations

### 1. Validation Location

**Current:** Package-level validation functions
**Target:** CRD schema + webhook validation

**Migration:**
- Keep validation package for reuse
- Use in validating webhooks
- Use in controller validation
- Generate OpenAPI schema from validation rules

### 2. OpenAPI Schema Generation

**Current:** Manual validation in code
**Target:** OpenAPI schema for CRD validation

**Migration:**
- Generate OpenAPI schema with validation rules
- Use pattern matching for FQDN validation
- Use format validation for domains
- Kubernetes validates on resource creation

### 3. Validating Webhooks

**Current:** Validation in application code
**Target:** Validating webhooks for complex validation

**Migration:**
- Implement validating webhook
- Use validation package in webhook
- Return user-friendly error messages
- Handle validation errors gracefully

### 4. Error Messages

**Current:** Error messages in validation functions
**Target:** Kubernetes API error messages

**Migration:**
- Format errors for Kubernetes API
- Use Kubernetes error types
- Provide field-level error messages
- Include validation context

### 5. Validation Reuse

**Current:** Public package for external use
**Target:** Continue as public package

**Migration:**
- Maintain validation package
- Use in webhooks and controllers
- Keep public API stable
- Document validation rules

## OpenAPI Schema Example

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
spec:
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            fqdn:
              type: string
              pattern: '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$'
              description: Fully Qualified Domain Name
```

## Validating Webhook Example

```go
func (v *DNSRecordValidator) ValidateCreate(ctx context.Context, obj runtime.Object) error {
    dnsRecord := obj.(*DNSRecord)
    
    if err := validation.ValidateFQDN(dnsRecord.Spec.FQDN); err != nil {
        return apierrors.NewInvalid(
            dnsRecord.GroupVersionKind().GroupKind(),
            dnsRecord.Name,
            field.ErrorList{
                field.Invalid(field.NewPath("spec", "fqdn"), dnsRecord.Spec.FQDN, err.Error()),
            },
        )
    }
    
    return nil
}
```

## Testing Strategy

### Current Testing Approach
- Unit tests for validation functions
- Edge case testing
- Format validation tests

### Target Testing Approach
- Continue unit tests
- Webhook validation tests
- OpenAPI schema validation tests
- Integration tests with testenv

## Summary

The Validation domain provides DNS and domain name validation utilities. Migration to Kubernetes will:

1. **Maintain Validation Package** - Keep as public package for reuse
2. **OpenAPI Schema** - Generate OpenAPI schema with validation patterns
3. **Validating Webhooks** - Use validation package in webhooks
4. **Controller Validation** - Use validation in controller reconciliation
5. **Error Formatting** - Format errors for Kubernetes API

The domain's validation logic should be preserved and reused in webhooks and controllers, with OpenAPI schema providing basic format validation at the Kubernetes API level.

