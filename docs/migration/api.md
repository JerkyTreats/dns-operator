# API Domain Migration Guide

## Overview

This guide provides step-by-step instructions for migrating the API domain from the current HTTP handler-based architecture to a Kubernetes-native CRD and controller pattern.

## Migration Summary

**From:** HTTP REST API with handler registry pattern  
**To:** Kubernetes CRDs with controller reconciliation pattern

### Key Changes
- HTTP handlers → Kubernetes controllers
- HTTP routes → Kubernetes API endpoints
- Request validation → CRD OpenAPI schema + webhooks
- JSON responses → CRD status updates

## Prerequisites

Before starting the migration, ensure you have:
- Kubernetes cluster access (v1.20+)
- kubectl configured
- Go 1.19+ installed
- Controller-runtime library knowledge
- Understanding of Kubernetes CRDs and controllers

## Current Architecture

### Components to Migrate

1. **HandlerRegistry** (`internal/api/handler/handler.go`)
   - Manages HTTP handlers
   - Routes requests to appropriate handlers
   - Initializes handler dependencies

2. **RecordHandler** (`internal/api/handler/record.go`)
   - `/add-record` → Create DNSRecord CRD
   - `/list-records` → `kubectl get dnsrecords`
   - `/remove-record` → Delete DNSRecord CRD

3. **RouteRegistry** (`internal/api/handler/registry.go`)
   - Centralized route registration
   - Route metadata for OpenAPI generation

4. **HealthHandler** (`internal/healthcheck/handler.go`)
   - `/health` endpoint
   - Can remain as HTTP endpoint or become liveness/readiness probes

5. **DeviceHandler** (`internal/tailscale/handler/handler.go`)
   - Tailscale device management
   - May remain as HTTP API or become CRD-based

## Target Architecture

### Option 1: Pure Kubernetes Native (Recommended)

```
┌─────────────────────────────────────────────────────────────┐
│              Kubernetes API Server                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ DNSRecord    │  │ Certificate  │  │ Device       │      │
│  │ CRD          │  │ CRD          │  │ CRD          │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│              DNS Operator Controllers                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ DNSRecord    │  │ Certificate  │  │ Device       │      │
│  │ Controller   │  │ Controller   │  │ Controller   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

### Option 2: Hybrid (Optional Convenience Layer)

Maintain REST API as convenience layer that creates/updates CRDs:
- REST API endpoints create CRDs
- Controllers reconcile CRDs
- Provides backward compatibility

## Step-by-Step Migration

### Phase 1: Define CRD Schema

#### 1.1 Create DNSRecord CRD Definition

Create `api/v1alpha1/dnsrecord_types.go`:

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSRecordSpec defines the desired state of DNSRecord
type DNSRecordSpec struct {
	Domain   string `json:"domain"`
	Type     string `json:"type"` // A, AAAA, CNAME, etc.
	Value    string `json:"value"`
	TTL      int    `json:"ttl,omitempty"`
	Proxy    bool   `json:"proxy,omitempty"`
}

// DNSRecordStatus defines the observed state of DNSRecord
type DNSRecordStatus struct {
	Phase      string `json:"phase,omitempty"` // Pending, Ready, Error
	Message    string `json:"message,omitempty"`
	LastUpdate string `json:"lastUpdate,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Domain",type="string",JSONPath=".spec.domain"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// DNSRecord is the Schema for the dnsrecords API
type DNSRecord struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSRecordSpec   `json:"spec,omitempty"`
	Status DNSRecordStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DNSRecordList contains a list of DNSRecord
type DNSRecordList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSRecord `json:"items"`
}
```

#### 1.2 Generate CRD Manifests

Use kubebuilder or controller-gen to generate CRD manifests:

```bash
# Install kubebuilder
# Follow: https://kubebuilder.io/quick-start.html

# Generate CRD manifests
make manifests
```

### Phase 2: Create Controller

#### 2.1 Create DNSRecord Controller

Create `internal/controller/dnsrecord_controller.go`:

```go
package controller

import (
	"context"
	"fmt"
	
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	
	"github.com/jerkytreats/dns-operator/api/v1alpha1"
	"github.com/jerkytreats/dns-operator/internal/dns/record"
)

// DNSRecordReconciler reconciles a DNSRecord object
type DNSRecordReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	RecordService *record.Service
}

// +kubebuilder:rbac:groups=dns.jerkytreats.io,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.jerkytreats.io,resources=dnsrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.jerkytreats.io,resources=dnsrecords/finalizers,verbs=update

func (r *DNSRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	
	var dnsRecord v1alpha1.DNSRecord
	if err := r.Get(ctx, req.NamespacedName, &dnsRecord); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	
	// Handle deletion
	if !dnsRecord.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &dnsRecord)
	}
	
	// Handle creation/update
	return r.handleReconciliation(ctx, &dnsRecord)
}

func (r *DNSRecordReconciler) handleReconciliation(ctx context.Context, dnsRecord *v1alpha1.DNSRecord) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	
	// Convert CRD spec to record service input
	recordInput := &record.CreateInput{
		Domain: dnsRecord.Spec.Domain,
		Type:   dnsRecord.Spec.Type,
		Value:  dnsRecord.Spec.Value,
		TTL:    dnsRecord.Spec.TTL,
		Proxy:  dnsRecord.Spec.Proxy,
	}
	
	// Create record via existing service
	if err := r.RecordService.CreateRecord(ctx, recordInput); err != nil {
		dnsRecord.Status.Phase = "Error"
		dnsRecord.Status.Message = err.Error()
		r.Status().Update(ctx, dnsRecord)
		return ctrl.Result{}, err
	}
	
	// Update status
	dnsRecord.Status.Phase = "Ready"
	dnsRecord.Status.Message = "DNS record created successfully"
	r.Status().Update(ctx, dnsRecord)
	
	return ctrl.Result{}, nil
}

func (r *DNSRecordReconciler) handleDeletion(ctx context.Context, dnsRecord *v1alpha1.DNSRecord) (ctrl.Result, error) {
	// Remove record via existing service
	if err := r.RecordService.RemoveRecord(ctx, dnsRecord.Spec.Domain); err != nil {
		return ctrl.Result{}, err
	}
	
	return ctrl.Result{}, nil
}

func (r *DNSRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DNSRecord{}).
		Complete(r)
}
```

#### 2.2 Register Controller in Main

Update `cmd/manager/main.go` (or create if doesn't exist):

```go
if err = (&controller.DNSRecordReconciler{
	Client:        mgr.GetClient(),
	Scheme:        mgr.GetScheme(),
	RecordService: recordService, // Initialize with dependencies
}).SetupWithManager(mgr); err != nil {
	setupLog.Error(err, "unable to create controller", "controller", "DNSRecord")
	os.Exit(1)
}
```

### Phase 3: Migrate Validation

#### 3.1 Move Validation to CRD Schema

Add OpenAPI validation markers to CRD types:

```go
// DNSRecordSpec defines the desired state of DNSRecord
type DNSRecordSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	Domain string `json:"domain"`
	
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=A;AAAA;CNAME;TXT;MX
	Type string `json:"type"`
	
	// +kubebuilder:validation:Required
	Value string `json:"value"`
	
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:default=300
	TTL int `json:"ttl,omitempty"`
	
	Proxy bool `json:"proxy,omitempty"`
}
```

#### 3.2 Create Validating Webhook (Optional)

For complex validation logic, create a validating webhook:

Create `api/v1alpha1/dnsrecord_webhook.go`:

```go
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/validate-dns-jerkytreats-io-v1alpha1-dnsrecord,mutating=false,failurePolicy=fail,sideEffects=None,groups=dns.jerkytreats.io,resources=dnsrecords,verbs=create;update,versions=v1alpha1,name=vdnsrecord.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &DNSRecord{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DNSRecord) ValidateCreate() error {
	// Add custom validation logic
	return r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DNSRecord) ValidateUpdate(old runtime.Object) error {
	// Add custom validation logic
	return r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DNSRecord) ValidateDelete() error {
	return nil
}

func (r *DNSRecord) validate() error {
	// Custom validation logic from handler
	return nil
}
```

### Phase 4: Remove HTTP Handler Layer

#### 4.1 Deprecate HandlerRegistry

Mark `HandlerRegistry` as deprecated:

```go
// Deprecated: HandlerRegistry is deprecated. Use Kubernetes CRDs and controllers instead.
// This will be removed in a future version.
type HandlerRegistry struct {
	// ... existing code
}
```

#### 4.2 Create Migration Path

For backward compatibility, create an adapter that converts HTTP requests to CRD operations:

Create `internal/api/adapter/http_to_crd.go`:

```go
package adapter

import (
	"context"
	"net/http"
	
	"sigs.k8s.io/controller-runtime/pkg/client"
	
	"github.com/jerkytreats/dns-operator/api/v1alpha1"
)

// HTTPToCRDAdapter adapts HTTP requests to CRD operations
type HTTPToCRDAdapter struct {
	Client client.Client
}

func (a *HTTPToCRDAdapter) AddRecord(w http.ResponseWriter, r *http.Request) {
	// Parse HTTP request
	// Create DNSRecord CRD
	// Return Kubernetes API response format
}
```

### Phase 5: Update API Server Command

#### 5.1 Modify Main Entry Point

Update `cmd/api/main.go` to support both modes:

```go
// Option 1: Run as controller manager
if os.Getenv("MODE") == "controller" {
	// Run controller manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Port:   9443,
	})
	// ... setup controllers
	mgr.Start(ctrl.SetupSignalHandler())
}

// Option 2: Run as HTTP API server (deprecated)
// ... existing HTTP server code
```

### Phase 6: Testing Migration

#### 6.1 Unit Tests

Create controller unit tests:

```go
func TestDNSRecordReconciler(t *testing.T) {
	// Use fake client
	// Test reconciliation logic
	// Test error handling
}
```

#### 6.2 Integration Tests

Test CRD operations:

```bash
# Create DNSRecord
kubectl apply -f - <<EOF
apiVersion: dns.jerkytreats.io/v1alpha1
kind: DNSRecord
metadata:
  name: example-record
spec:
  domain: example.com
  type: A
  value: 192.0.2.1
EOF

# Verify status
kubectl get dnsrecord example-record -o yaml

# Delete
kubectl delete dnsrecord example-record
```

## Migration Checklist

### Pre-Migration
- [ ] Review current handler implementations
- [ ] Document all HTTP endpoints and their behaviors
- [ ] Identify all validation rules
- [ ] List all dependencies for each handler

### CRD Definition
- [ ] Define CRD schema matching current request structures
- [ ] Add OpenAPI validation markers
- [ ] Generate CRD manifests
- [ ] Test CRD installation

### Controller Implementation
- [ ] Create controller for each resource type
- [ ] Implement reconciliation logic
- [ ] Handle creation, update, deletion
- [ ] Update CRD status appropriately
- [ ] Add error handling and retries

### Validation Migration
- [ ] Move validation to CRD schema
- [ ] Create validating webhooks if needed
- [ ] Test validation rules
- [ ] Verify error messages

### Handler Removal
- [ ] Mark handlers as deprecated
- [ ] Create migration adapter (if needed)
- [ ] Update documentation
- [ ] Remove handler code (after migration period)

### Testing
- [ ] Unit tests for controllers
- [ ] Integration tests with testenv
- [ ] End-to-end tests
- [ ] Performance testing

### Documentation
- [ ] Update API documentation
- [ ] Create CRD usage examples
- [ ] Update deployment guides
- [ ] Document migration path for users

## Rollback Plan

If issues arise during migration:

1. **Keep HTTP handlers active** during transition period
2. **Run both systems in parallel** (HTTP API + CRDs)
3. **Use feature flags** to switch between modes
4. **Maintain backward compatibility** adapter layer

## Post-Migration

After successful migration:

1. **Remove deprecated HTTP handlers** (after grace period)
2. **Remove HandlerRegistry** code
3. **Update all documentation** to use kubectl/CRDs
4. **Update client libraries** to use Kubernetes client
5. **Monitor controller metrics** and performance

## Additional Considerations

### Health Checks

The `/health` endpoint can be converted to:
- Kubernetes liveness probe: `/healthz`
- Kubernetes readiness probe: `/readyz`
- Or keep as HTTP endpoint for external monitoring

### OpenAPI Documentation

Instead of generating OpenAPI from routes:
- Use CRD OpenAPI schema
- Generate from CRD definitions
- Kubernetes API server provides OpenAPI spec

### Authentication/Authorization

Replace HTTP auth with:
- Kubernetes RBAC
- ServiceAccount-based authentication
- Webhook authentication if needed

## References

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Controller Runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Kubernetes CRDs](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/)
- [Validating Webhooks](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook)

