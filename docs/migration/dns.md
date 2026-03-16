# DNS Domain Migration Guide

## Overview

This guide provides step-by-step instructions for migrating the DNS domain from the current file-based zone file system to a Kubernetes-native operator pattern using Custom Resource Definitions (CRDs) and ConfigMaps.

## Migration Goals

- Replace file-based zone files with ConfigMap-based storage
- Migrate from HTTP API handlers to Kubernetes controllers
- Convert DNS record management to DNSRecord CRDs
- Move Corefile generation to ConfigMap-based approach
- Preserve zone serial number management
- Maintain DNS record validation and normalization logic

## Prerequisites

- Kubernetes cluster (v1.20+)
- kubebuilder installed
- controller-runtime v0.15.0+
- Access to existing DNS zone files (for data migration)
- Understanding of CoreDNS zone file format

## Migration Steps

### Step 1: Define DNSRecord CRD

Create the DNSRecord Custom Resource Definition with proper validation schema.

**File:** `api/dns/v1alpha1/dnsrecord_types.go`

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DNSRecordSpec defines the desired state of DNSRecord
type DNSRecordSpec struct {
	// Name is the DNS record name (without domain)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`
	Name string `json:"name"`

	// Domain is the DNS domain for this record
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`
	Domain string `json:"domain"`

	// Type is the DNS record type (A, AAAA, CNAME, etc.)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=A;AAAA;CNAME;TXT
	// +kubebuilder:default=A
	Type string `json:"type,omitempty"`

	// TargetIP is the IP address for A/AAAA records
	// +kubebuilder:validation:Pattern=`^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$|^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`
	TargetIP string `json:"targetIP,omitempty"`

	// TargetHost is the hostname for CNAME records
	TargetHost string `json:"targetHost,omitempty"`

	// TTL is the time-to-live for the DNS record
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:default=300
	TTL int `json:"ttl,omitempty"`

	// TailscaleDeviceRef is an optional reference to a TailscaleDevice for IP resolution
	TailscaleDeviceRef *TailscaleDeviceRef `json:"tailscaleDeviceRef,omitempty"`

	// Proxy configuration for reverse proxy rules
	Proxy *ProxyConfig `json:"proxy,omitempty"`
}

// TailscaleDeviceRef references a TailscaleDevice for IP resolution
type TailscaleDeviceRef struct {
	// Name of the TailscaleDevice resource
	Name string `json:"name"`
	// Namespace of the TailscaleDevice resource
	Namespace string `json:"namespace,omitempty"`
}

// ProxyConfig defines reverse proxy configuration
type ProxyConfig struct {
	Enabled    bool   `json:"enabled,omitempty"`
	TargetPort int    `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// DNSRecordStatus defines the observed state of DNSRecord
type DNSRecordStatus struct {
	// FQDN is the fully qualified domain name
	FQDN string `json:"fqdn,omitempty"`

	// ZoneFile is the name of the ConfigMap containing the zone file
	ZoneFile string `json:"zoneFile,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="FQDN",type=string,JSONPath=`.status.fqdn`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetIP`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

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
	metav1.ListMeta  `json:"metadata,omitempty"`
	Items            []DNSRecord `json:"items"`
}
```

**Key Points:**
- Use OpenAPI schema validation for basic format checks
- Include proper kubebuilder markers for CRD generation
- Support both direct IP and TailscaleDevice references
- Include proxy configuration in the spec

### Step 2: Implement DNSRecord Controller

Create the controller that reconciles DNSRecord resources and manages zone ConfigMaps.

**File:** `internal/controller/dnsrecord_controller.go`

```go
package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dnsv1alpha1 "github.com/jerkytreats/dns-operator/api/dns/v1alpha1"
)

// DNSRecordReconciler reconciles a DNSRecord object
type DNSRecordReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dns.jerkytreats.dev,resources=dnsrecords,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.jerkytreats.dev,resources=dnsrecords/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.jerkytreats.dev,resources=dnsrecords/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *DNSRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var dnsRecord dnsv1alpha1.DNSRecord
	if err := r.Get(ctx, req.NamespacedName, &dnsRecord); err != nil {
		if apierrors.IsNotFound(err) {
			// DNSRecord deleted, reconcile zone to remove record
			return r.reconcileZone(ctx, req.Namespace, "")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Reconcile the zone for this DNSRecord's domain
	return r.reconcileZone(ctx, dnsRecord.Namespace, dnsRecord.Spec.Domain)
}

func (r *DNSRecordReconciler) reconcileZone(ctx context.Context, namespace, domain string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// List all DNSRecords for this domain
	var dnsRecords dnsv1alpha1.DNSRecordList
	if err := r.List(ctx, &dnsRecords, client.InNamespace(namespace)); err != nil {
		return ctrl.Result{}, err
	}

	// Filter records for this domain
	var domainRecords []dnsv1alpha1.DNSRecord
	for _, record := range dnsRecords.Items {
		if record.Spec.Domain == domain {
			domainRecords = append(domainRecords, record)
		}
	}

	// Generate zone file content
	zoneContent, err := r.generateZoneFile(ctx, domain, domainRecords)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create or update zone ConfigMap
	zoneConfigMapName := fmt.Sprintf("zone-%s", strings.ReplaceAll(domain, ".", "-"))
	zoneConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zoneConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"dns.jerkytreats.dev/zone": domain,
			},
		},
		Data: map[string]string{
			"zone": zoneContent,
		},
	}

	if err := ctrl.SetControllerReference(&dnsRecords.Items[0], zoneConfigMap, r.Scheme); err == nil {
		// Set owner reference if we have records
	}

	if err := r.Create(ctx, zoneConfigMap); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := r.Update(ctx, zoneConfigMap); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	}

	// Update DNSRecord statuses
	for _, record := range domainRecords {
		record.Status.FQDN = fmt.Sprintf("%s.%s", record.Spec.Name, record.Spec.Domain)
		record.Status.ZoneFile = zoneConfigMapName
		record.Status.Conditions = []metav1.Condition{
			{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "ZoneReconciled",
			},
		}
		if err := r.Status().Update(ctx, &record); err != nil {
			logger.Error(err, "failed to update DNSRecord status", "record", record.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *DNSRecordReconciler) generateZoneFile(ctx context.Context, domain string, records []dnsv1alpha1.DNSRecord) (string, error) {
	var zone strings.Builder

	// Generate SOA record
	serial := generateSerial()
	zone.WriteString(fmt.Sprintf("$ORIGIN %s.\n", domain))
	zone.WriteString(fmt.Sprintf("$TTL 300\n\n"))
	zone.WriteString(fmt.Sprintf("@\tIN\tSOA\tns1.%s. admin.%s. %s 3600 1800 604800 86400\n", domain, domain, serial))
	zone.WriteString(fmt.Sprintf("@\tIN\tNS\tns1.%s.\n\n", domain))

	// Sort records by name for consistent output
	sort.Slice(records, func(i, j int) bool {
		return records[i].Spec.Name < records[j].Spec.Name
	})

	// Generate A records
	for _, record := range records {
		if record.Spec.Type == "A" && record.Spec.TargetIP != "" {
			ttl := record.Spec.TTL
			if ttl == 0 {
				ttl = 300
			}
			zone.WriteString(fmt.Sprintf("%s\t%d\tIN\tA\t%s\n", record.Spec.Name, ttl, record.Spec.TargetIP))
		}
	}

	return zone.String(), nil
}

func generateSerial() string {
	// Use timestamp-based serial (YYYYMMDDHH format)
	return time.Now().Format("2006010215")
}
```

**Key Points:**
- Controller aggregates all DNSRecords by domain
- Generates complete zone file from all records
- Updates zone ConfigMap atomically
- Updates DNSRecord status with FQDN and zone file reference

### Step 3: Migrate Zone File Storage

Replace file-based zone storage with ConfigMap-based storage.

**Migration Strategy:**

1. **Export existing zone files:**
   ```bash
   # Export all zone files from current system
   kubectl create configmap zone-export --from-file=/etc/coredns/zones/ --dry-run=client -o yaml > zones-export.yaml
   ```

2. **Convert zone files to DNSRecord CRDs:**
   Create a migration script to parse zone files and generate DNSRecord manifests:

   ```go
   // internal/migration/zone_to_crd.go
   package migration

   func ConvertZoneFileToDNSRecords(zoneFilePath, domain string) ([]dnsv1alpha1.DNSRecord, error) {
       // Parse zone file
       // Extract A records
       // Create DNSRecord CRDs
       // Return list of DNSRecords
   }
   ```

3. **Import zone files as ConfigMaps:**
   ```bash
   # Create ConfigMaps from exported zone files
   kubectl apply -f zones-export.yaml
   ```

### Step 4: Migrate Corefile Generation

Move Corefile generation from template-based file system to ConfigMap-based approach.

**File:** `internal/controller/corefile_controller.go`

```go
package controller

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dnsv1alpha1 "github.com/jerkytreats/dns-operator/api/dns/v1alpha1"
)

// CorefileReconciler manages CoreDNS Corefile ConfigMap
type CorefileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *CorefileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// List all DNSRecords to get unique domains
	var dnsRecords dnsv1alpha1.DNSRecordList
	if err := r.List(ctx, &dnsRecords); err != nil {
		return ctrl.Result{}, err
	}

	// Extract unique domains
	domains := make(map[string]bool)
	for _, record := range dnsRecords.Items {
		domains[record.Spec.Domain] = true
	}

	// Generate Corefile
	corefileContent, err := r.generateCorefile(domains)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create or update Corefile ConfigMap
	corefileConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns-corefile",
			Namespace: "dns-system",
		},
		Data: map[string]string{
			"Corefile": corefileContent,
		},
	}

	if err := r.Create(ctx, corefileConfigMap); err != nil {
		if apierrors.IsAlreadyExists(err) {
			if err := r.Update(ctx, corefileConfigMap); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *CorefileReconciler) generateCorefile(domains map[string]bool) (string, error) {
	var corefile strings.Builder

	corefile.WriteString(".:53 {\n")
	corefile.WriteString("    errors\n")
	corefile.WriteString("    health\n")
	corefile.WriteString("    ready\n")
	corefile.WriteString("    forward . /etc/resolv.conf\n")
	corefile.WriteString("    cache 30\n")
	corefile.WriteString("    loop\n")
	corefile.WriteString("    reload\n")
	corefile.WriteString("    loadbalance\n")
	corefile.WriteString("}\n\n")

	// Generate domain blocks
	for domain := range domains {
		zoneConfigMapName := fmt.Sprintf("zone-%s", strings.ReplaceAll(domain, ".", "-"))
		corefile.WriteString(fmt.Sprintf("%s:53 {\n", domain))
		corefile.WriteString("    file /etc/coredns/zones/" + zoneConfigMapName + "/zone\n")
		corefile.WriteString("    errors\n")
		corefile.WriteString("    log\n")
		corefile.WriteString("}\n\n")
	}

	return corefile.String(), nil
}
```

**Key Points:**
- Generate Corefile from all DNSRecord domains
- Reference zone ConfigMaps in Corefile
- Update Corefile when domains change

### Step 5: Update CoreDNS Deployment

Configure CoreDNS to use ConfigMap-mounted zone files.

**File:** `config/coredns/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: coredns
  namespace: dns-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: coredns
  template:
    metadata:
      labels:
        app: coredns
    spec:
      containers:
      - name: coredns
        image: coredns/coredns:latest
        volumeMounts:
        - name: config-volume
          mountPath: /etc/coredns
          readOnly: true
        - name: zones-volume
          mountPath: /etc/coredns/zones
          readOnly: true
      volumes:
      - name: config-volume
        configMap:
          name: coredns-corefile
      - name: zones-volume
        # Use projected volume to mount multiple zone ConfigMaps
        projected:
          sources:
          - configMap:
              name: zone-internal-jerkytreats-dev
          - configMap:
              name: zone-example-com
```

**Key Points:**
- Mount Corefile ConfigMap
- Mount zone ConfigMaps as projected volume
- Use read-only mounts for security

### Step 6: Migrate Validation Logic

Move validation from service layer to CRD schema and webhooks.

**File:** `api/dns/v1alpha1/dnsrecord_webhook.go`

```go
package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/jerkytreats/dns-operator/pkg/validation"
)

// +kubebuilder:webhook:path=/mutate-dns-jerkytreats-dev-v1alpha1-dnsrecord,mutating=true,failurePolicy=fail,sideEffects=None,groups=dns.jerkytreats.dev,resources=dnsrecords,verbs=create;update,versions=v1alpha1,name=mdnsrecord.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &DNSRecord{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DNSRecord) Default() {
	if r.Spec.Type == "" {
		r.Spec.Type = "A"
	}
	if r.Spec.TTL == 0 {
		r.Spec.TTL = 300
	}
	// Normalize name to lowercase
	r.Spec.Name = strings.ToLower(strings.TrimSpace(r.Spec.Name))
}

// +kubebuilder:webhook:path=/validate-dns-jerkytreats-dev-v1alpha1-dnsrecord,mutating=false,failurePolicy=fail,sideEffects=None,groups=dns.jerkytreats.dev,resources=dnsrecords,verbs=create;update,versions=v1alpha1,name=vdnsrecord.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &DNSRecord{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DNSRecord) ValidateCreate() (admission.Warnings, error) {
	return r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DNSRecord) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DNSRecord) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (r *DNSRecord) validate() (admission.Warnings, error) {
	// Use existing validation package
	if err := validation.ValidateDNSName(r.Spec.Name); err != nil {
		return nil, fmt.Errorf("invalid DNS name: %w", err)
	}

	if err := validation.ValidateDomain(r.Spec.Domain); err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}

	// Validate FQDN
	fqdn := fmt.Sprintf("%s.%s", r.Spec.Name, r.Spec.Domain)
	if err := validation.ValidateFQDN(fqdn); err != nil {
		return nil, fmt.Errorf("invalid FQDN: %w", err)
	}

	// Validate record type-specific fields
	if r.Spec.Type == "A" && r.Spec.TargetIP == "" {
		return nil, fmt.Errorf("targetIP is required for A records")
	}

	return nil, nil
}
```

**Key Points:**
- Use mutating webhook for normalization
- Use validating webhook for business logic validation
- Reuse existing validation package functions

### Step 7: Data Migration Process

**Step 7.1: Export Current State**

```bash
# Export all zone files
kubectl exec -n dns-system dns-manager-pod -- tar czf /tmp/zones.tar.gz /etc/coredns/zones/
kubectl cp dns-system/dns-manager-pod:/tmp/zones.tar.gz ./zones-backup.tar.gz
```

**Step 7.2: Parse Zone Files**

Create a migration tool to convert zone files to DNSRecord CRDs:

```go
// cmd/migrate/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	dnsv1alpha1 "github.com/jerkytreats/dns-operator/api/dns/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	zonesDir := os.Args[1]
	outputDir := os.Args[2]

	zoneFiles, err := filepath.Glob(filepath.Join(zonesDir, "*.zone"))
	if err != nil {
		panic(err)
	}

	for _, zoneFile := range zoneFiles {
		domain := strings.TrimSuffix(filepath.Base(zoneFile), ".zone")
		records := parseZoneFile(zoneFile, domain)
		
		for _, record := range records {
			outputFile := filepath.Join(outputDir, fmt.Sprintf("%s-%s.yaml", domain, record.Spec.Name))
			writeDNSRecord(outputFile, record)
		}
	}
}

func parseZoneFile(zoneFile, domain string) []dnsv1alpha1.DNSRecord {
	// Parse zone file and extract A records
	// Return list of DNSRecord CRDs
}
```

**Step 7.3: Apply DNSRecord CRDs**

```bash
# Apply all DNSRecord CRDs
kubectl apply -f migrated-records/
```

**Step 7.4: Verify Migration**

```bash
# Check DNSRecord resources
kubectl get dnsrecords

# Check zone ConfigMaps
kubectl get configmaps -l dns.jerkytreats.dev/zone

# Verify Corefile
kubectl get configmap coredns-corefile -o yaml

# Test DNS resolution
dig @coredns-service app.internal.example.test
```

### Step 8: Update Dependencies

Remove file-based dependencies and update imports.

**Changes Required:**

1. **Remove file system dependencies:**
   - Remove `internal/dns/coredns/manager.go` file operations
   - Remove zone file path configuration
   - Remove file-based serial number management

2. **Update service layer:**
   - Remove `internal/dns/record/service.go` HTTP handler logic
   - Keep validation and normalization logic for webhook use

3. **Update configuration:**
   - Remove zone file path configuration keys
   - Remove Corefile template path configuration
   - Add ConfigMap namespace configuration

## Rollback Procedure

If migration fails, follow these steps to rollback:

1. **Stop the operator:**
   ```bash
   kubectl scale deployment dns-operator --replicas=0
   ```

2. **Restore zone files:**
   ```bash
   kubectl cp ./zones-backup.tar.gz dns-system/dns-manager-pod:/tmp/zones.tar.gz
   kubectl exec -n dns-system dns-manager-pod -- tar xzf /tmp/zones.tar.gz -C /
   ```

3. **Restart original DNS manager:**
   ```bash
   kubectl scale deployment dns-manager --replicas=1
   ```

4. **Verify DNS resolution:**
   ```bash
   dig @dns-manager-service app.internal.example.test
   ```

## Testing Checklist

- [ ] DNSRecord CRD created and validated
- [ ] Controller reconciles DNSRecord resources
- [ ] Zone ConfigMaps created with correct content
- [ ] Corefile ConfigMap generated correctly
- [ ] CoreDNS deployment mounts ConfigMaps
- [ ] DNS resolution works for migrated records
- [ ] Serial numbers update correctly
- [ ] Validation webhooks work correctly
- [ ] Mutating webhooks normalize input
- [ ] Controller handles record deletion
- [ ] Controller handles domain changes
- [ ] Multiple zones work correctly
- [ ] Rollback procedure tested

## Common Issues and Solutions

### Issue: Zone ConfigMap not updating

**Solution:** Ensure controller watches ConfigMaps and triggers reconciliation on changes.

### Issue: Serial number conflicts

**Solution:** Use atomic ConfigMap updates and coordinate serial updates in controller.

### Issue: CoreDNS not reloading

**Solution:** Ensure CoreDNS deployment has reload plugin enabled and watches mounted ConfigMaps.

### Issue: DNS resolution fails after migration

**Solution:** 
1. Verify zone ConfigMap content matches original zone file
2. Check Corefile references correct zone paths
3. Verify CoreDNS can read mounted ConfigMaps
4. Check CoreDNS logs for errors

### Issue: Validation webhook rejects valid records

**Solution:** 
1. Check webhook logs for validation errors
2. Verify validation package functions work correctly
3. Test webhook with `kubectl create --dry-run=server`

## Post-Migration Tasks

1. **Monitor DNS resolution:**
   - Set up DNS monitoring
   - Alert on resolution failures
   - Track zone update frequency

2. **Optimize controller:**
   - Add controller metrics
   - Optimize reconciliation logic
   - Add rate limiting if needed

3. **Documentation:**
   - Update API documentation
   - Document DNSRecord CRD usage
   - Create user guides

4. **Cleanup:**
   - Remove old file-based code
   - Remove HTTP API handlers
   - Clean up unused configuration

## Summary

The DNS domain migration transforms the system from file-based zone management to Kubernetes-native CRD-based management. Key changes:

1. **DNSRecord CRD** replaces HTTP API for record management
2. **ConfigMap zone files** replace file system zone files
3. **Controller reconciliation** replaces HTTP handlers
4. **Webhook validation** replaces service-level validation
5. **ConfigMap Corefile** replaces template-based Corefile generation

The migration preserves all existing functionality while providing Kubernetes-native declarative management and better integration with the Kubernetes ecosystem.
