# Proxy Domain Migration Guide

## Overview

This guide provides step-by-step instructions for migrating the Proxy domain from the reference implementation (file-based storage, supervisord reload) to a Kubernetes-native operator pattern using CRDs, ConfigMaps, and controller-based reconciliation.

### Migration Goals

- Replace JSON file storage with ProxyRule CRDs stored in etcd
- Move Caddyfile generation to a ConfigMap managed by a controller
- Replace supervisord-based reload with Caddy admin API or ConfigMap watching
- Deploy Caddy as a separate Kubernetes Deployment/DaemonSet
- Maintain backward compatibility during migration
- Preserve all existing proxy rule functionality

### Current State

**Reference Implementation:**
- Location: `reference/internal/proxy/`
- Storage: JSON file (`data/proxy_rules.json`)
- Configuration: Template-based Caddyfile generation
- Reload: Supervisord (`supervisorctl reread && supervisorctl update`)
- Deployment: Unified container with supervisord

### Target State

**Kubernetes Operator:**
- Storage: ProxyRule CRDs in etcd
- Configuration: ConfigMap containing generated Caddyfile
- Reload: Caddy admin API or ConfigMap file watching
- Deployment: Separate Caddy Deployment/DaemonSet
- Management: Controller-based reconciliation

## Pre-Migration Checklist

Before starting the migration, ensure:

- [ ] Kubernetes cluster is accessible and configured
- [ ] kubebuilder or operator-sdk is installed
- [ ] Existing proxy rules are documented/backed up
- [ ] Caddy template file is available (`/etc/caddy/Caddyfile.template`)
- [ ] Access to existing proxy configuration for reference
- [ ] Test environment available for validation

## Step 1: Define ProxyRule CRD

### 1.1 Create CRD API Definition

Create the ProxyRule CRD API types:

**File:** `api/proxy/v1alpha1/proxyrule_types.go`

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProxyRuleSpec defines the desired state of ProxyRule
type ProxyRuleSpec struct {
	// Hostname is the FQDN for the reverse proxy rule
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`
	Hostname string `json:"hostname"`

	// TargetIP is the IP address to proxy to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$`
	TargetIP string `json:"targetIP"`

	// TargetPort is the port to proxy to (1-65535)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	TargetPort int `json:"targetPort"`

	// Protocol is the protocol to use (http or https)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=http;https
	Protocol string `json:"protocol"`

	// Enabled determines if the proxy rule is active
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
}

// ProxyRuleStatus defines the observed state of ProxyRule
type ProxyRuleStatus struct {
	// State indicates the current state of the proxy rule
	// +kubebuilder:validation:Enum=Pending;Active;Error
	State string `json:"state,omitempty"`

	// Message provides additional information about the state
	Message string `json:"message,omitempty"`

	// LastUpdated is the timestamp of the last status update
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Hostname",type="string",JSONPath=".spec.hostname"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetIP"
// +kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.targetPort"
// +kubebuilder:printcolumn:name="Protocol",type="string",JSONPath=".spec.protocol"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ProxyRule is the Schema for the proxyrules API
type ProxyRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProxyRuleSpec   `json:"spec,omitempty"`
	Status ProxyRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProxyRuleList contains a list of ProxyRule
type ProxyRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Items           []ProxyRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProxyRule{}, &ProxyRuleList{})
}
```

### 1.2 Generate CRD Manifests

Run kubebuilder to generate CRD manifests:

```bash
make manifests
```

This generates the CRD YAML in `config/crd/bases/`.

### 1.3 Install CRD

Install the CRD in your cluster:

```bash
kubectl apply -f config/crd/bases/proxy.jerkytreats.dev_proxyrules.yaml
```

Verify installation:

```bash
kubectl get crd proxyrules.proxy.jerkytreats.dev
```

## Step 2: Implement ProxyRule Controller

### 2.1 Create Controller Structure

**File:** `internal/controller/proxyrule_controller.go`

```go
package controller

import (
	"context"
	"fmt"
	"text/template"
	"bytes"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	proxyv1alpha1 "github.com/jerkytreats/dns-operator/api/proxy/v1alpha1"
)

// ProxyRuleReconciler reconciles a ProxyRule object
type ProxyRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	
	// ConfigMap name for Caddyfile
	caddyConfigMapName string
	caddyConfigMapNS   string
	
	// Template for Caddyfile generation
	caddyTemplate *template.Template
}

// +kubebuilder:rbac:groups=proxy.jerkytreats.dev,resources=proxyrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=proxy.jerkytreats.dev,resources=proxyrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=proxy.jerkytreats.dev,resources=proxyrules/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *ProxyRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch all ProxyRules
	var proxyRuleList proxyv1alpha1.ProxyRuleList
	if err := r.List(ctx, &proxyRuleList); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list ProxyRules: %w", err)
	}

	// Aggregate active rules
	activeRules := make([]proxyv1alpha1.ProxyRule, 0)
	for _, rule := range proxyRuleList.Items {
		if rule.Spec.Enabled {
			activeRules = append(activeRules, rule)
		}
	}

	// Generate Caddyfile
	caddyfile, err := r.generateCaddyfile(activeRules)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to generate Caddyfile: %w", err)
	}

	// Update ConfigMap
	if err := r.updateConfigMap(ctx, caddyfile); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	// Update status for all rules
	for i := range proxyRuleList.Items {
		rule := &proxyRuleList.Items[i]
		if err := r.updateRuleStatus(ctx, rule, activeRules); err != nil {
			logger.Error(err, "failed to update rule status", "rule", rule.Name)
		}
	}

	logger.Info("Reconciled proxy rules", "active", len(activeRules))
	return ctrl.Result{}, nil
}

func (r *ProxyRuleReconciler) generateCaddyfile(rules []proxyv1alpha1.ProxyRule) (string, error) {
	templateData := struct {
		Rules      []proxyv1alpha1.ProxyRule
		GeneratedAt string
	}{
		Rules:      rules,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	var buf bytes.Buffer
	if err := r.caddyTemplate.Execute(&buf, templateData); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func (r *ProxyRuleReconciler) updateConfigMap(ctx context.Context, caddyfile string) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.caddyConfigMapName,
			Namespace: r.caddyConfigMapNS,
		},
	}

	op, err := ctrl.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		if configMap.Data == nil {
			configMap.Data = make(map[string]string)
		}
		configMap.Data["Caddyfile"] = caddyfile
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update ConfigMap: %w", err)
	}

	logger := log.FromContext(ctx)
	logger.Info("ConfigMap updated", "operation", op, "name", r.caddyConfigMapName)
	return nil
}

func (r *ProxyRuleReconciler) updateRuleStatus(ctx context.Context, rule *proxyv1alpha1.ProxyRule, activeRules []proxyv1alpha1.ProxyRule) error {
	// Check if rule is in active rules
	isActive := false
	for _, active := range activeRules {
		if active.Name == rule.Name && active.Namespace == rule.Namespace {
			isActive = true
			break
		}
	}

	status := proxyv1alpha1.ProxyRuleStatus{
		LastUpdated: metav1.Now(),
	}

	if isActive {
		status.State = "Active"
		status.Message = "Proxy rule is active and included in Caddyfile"
	} else if !rule.Spec.Enabled {
		status.State = "Pending"
		status.Message = "Proxy rule is disabled"
	} else {
		status.State = "Error"
		status.Message = "Proxy rule not found in active rules"
	}

	rule.Status = status
	return r.Status().Update(ctx, rule)
}

func (r *ProxyRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&proxyv1alpha1.ProxyRule{}).
		Complete(r)
}
```

### 2.2 Initialize Controller in Main

**File:** `cmd/manager/main.go` (add to existing setup)

```go
// Add after other controller setup
proxyRuleReconciler := &controller.ProxyRuleReconciler{
	Client:             mgr.GetClient(),
	Scheme:             mgr.GetScheme(),
	caddyConfigMapName: "caddy-config",
	caddyConfigMapNS:   "default", // or from config
}

// Load Caddy template
templateContent, err := os.ReadFile("/etc/caddy/Caddyfile.template")
if err != nil {
	return fmt.Errorf("failed to read Caddy template: %w", err)
}

proxyRuleReconciler.caddyTemplate, err = template.New("caddyfile").Parse(string(templateContent))
if err != nil {
	return fmt.Errorf("failed to parse Caddy template: %w", err)
}

if err := proxyRuleReconciler.SetupWithManager(mgr); err != nil {
	return fmt.Errorf("unable to create ProxyRule controller: %w", err)
}
```

## Step 3: Deploy Caddy as Kubernetes Deployment

### 3.1 Create Caddy Deployment

**File:** `config/samples/caddy-deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: caddy
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: caddy
  template:
    metadata:
      labels:
        app: caddy
    spec:
      containers:
      - name: caddy
        image: caddy:latest
        ports:
        - containerPort: 80
          name: http
        - containerPort: 443
          name: https
        - containerPort: 2019
          name: admin
        volumeMounts:
        - name: caddy-config
          mountPath: /etc/caddy
          readOnly: true
        - name: caddy-data
          mountPath: /data
        - name: caddy-config-data
          mountPath: /config
        # Enable admin API for dynamic config reload
        args:
        - run
        - --config
        - /etc/caddy/Caddyfile
        - --adapter
        - caddyfile
        env:
        - name: CADDY_ADMIN
          value: "0.0.0.0:2019"
      volumes:
      - name: caddy-config
        configMap:
          name: caddy-config
      - name: caddy-data
        emptyDir: {}
      - name: caddy-config-data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: caddy
  namespace: default
spec:
  selector:
    app: caddy
  ports:
  - port: 80
    targetPort: 80
    name: http
  - port: 443
    targetPort: 443
    name: https
  type: LoadBalancer  # or ClusterIP with Ingress
```

### 3.2 Alternative: Use Caddy Admin API for Reload

If using Caddy admin API instead of ConfigMap watching, update the controller to trigger reload:

```go
func (r *ProxyRuleReconciler) reloadCaddy(ctx context.Context) error {
	// Use Caddy admin API to reload configuration
	// POST http://caddy:2019/load
	// This requires the ConfigMap to be mounted and Caddy to watch it
	// Or use the admin API to load the config directly
	
	// For ConfigMap watching, Caddy will auto-reload when the file changes
	// No additional action needed
	return nil
}
```

## Step 4: Data Migration

### 4.1 Export Existing Proxy Rules

Create a migration script to export existing rules from JSON storage:

**File:** `scripts/migrate-proxy-rules.sh`

```bash
#!/bin/bash
# Export proxy rules from JSON storage to ProxyRule CRDs

JSON_FILE="${1:-data/proxy_rules.json}"
NAMESPACE="${2:-default}"

if [ ! -f "$JSON_FILE" ]; then
    echo "Error: JSON file not found: $JSON_FILE"
    exit 1
fi

# Parse JSON and create ProxyRule CRDs
# This is a simplified example - adjust based on your JSON structure
jq -r '. | to_entries[] | @json' "$JSON_FILE" | while read -r rule_json; do
    hostname=$(echo "$rule_json" | jq -r '.value.hostname')
    target_ip=$(echo "$rule_json" | jq -r '.value.target_ip')
    target_port=$(echo "$rule_json" | jq -r '.value.target_port')
    protocol=$(echo "$rule_json" | jq -r '.value.protocol')
    enabled=$(echo "$rule_json" | jq -r '.value.enabled // true')
    
    # Create ProxyRule CRD
    cat <<EOF | kubectl apply -f -
apiVersion: proxy.jerkytreats.dev/v1alpha1
kind: ProxyRule
metadata:
  name: $(echo "$hostname" | tr '.' '-')
  namespace: $NAMESPACE
spec:
  hostname: $hostname
  targetIP: $target_ip
  targetPort: $target_port
  protocol: $protocol
  enabled: $enabled
EOF
done
```

### 4.2 Verify Migration

After migration, verify all rules are present:

```bash
kubectl get proxyrules -A
kubectl get proxyrules -o yaml
```

## Step 5: Update Caddyfile Template

Ensure the Caddyfile template works with the new structure:

**File:** `config/caddy/Caddyfile.template`

```
{{- range .Rules }}
{{ .Spec.Hostname }} {
    reverse_proxy {{ .Spec.TargetIP }}:{{ .Spec.TargetPort }}
}
{{- end }}
```

## Step 6: Testing

### 6.1 Unit Tests

Test the controller logic:

**File:** `internal/controller/proxyrule_controller_test.go`

```go
package controller

import (
	"testing"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	proxyv1alpha1 "github.com/jerkytreats/dns-operator/api/proxy/v1alpha1"
)

var _ = Describe("ProxyRuleController", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		client client.Client
		reconciler *ProxyRuleReconciler
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		_ = proxyv1alpha1.AddToScheme(scheme.Scheme)
		client = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		
		reconciler = &ProxyRuleReconciler{
			Client: client,
			Scheme: scheme.Scheme,
			caddyConfigMapName: "caddy-config",
			caddyConfigMapNS: "default",
		}
	})

	AfterEach(func() {
		cancel()
	})

	It("should reconcile ProxyRules and generate Caddyfile", func() {
		// Create test ProxyRule
		rule := &proxyv1alpha1.ProxyRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rule",
				Namespace: "default",
			},
			Spec: proxyv1alpha1.ProxyRuleSpec{
				Hostname:  "test.example.com",
				TargetIP:  "192.168.1.1",
				TargetPort: 8080,
				Protocol:  "http",
				Enabled:   true,
			},
		}
		Expect(client.Create(ctx, rule)).To(Succeed())

		// Reconcile
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKeyFromObject(rule),
		})
		Expect(err).NotTo(HaveOccurred())

		// Verify ConfigMap was created
		// Add assertions here
	})
})
```

### 6.2 Integration Tests

Test with a real Kubernetes cluster using envtest:

```go
// Use controller-runtime's envtest for integration testing
```

### 6.3 Manual Testing

1. Create a ProxyRule:
```bash
kubectl apply -f - <<EOF
apiVersion: proxy.jerkytreats.dev/v1alpha1
kind: ProxyRule
metadata:
  name: test-proxy
spec:
  hostname: test.example.com
  targetIP: 192.168.1.1
  targetPort: 8080
  protocol: http
  enabled: true
EOF
```

2. Verify ConfigMap is updated:
```bash
kubectl get configmap caddy-config -o yaml
```

3. Verify Caddy reloaded:
```bash
kubectl logs -l app=caddy
```

4. Test proxy functionality:
```bash
curl -H "Host: test.example.com" http://<caddy-service-ip>
```

## Step 7: Remove Old Implementation

Once migration is complete and verified:

1. **Remove old proxy manager code:**
   - Delete `reference/internal/proxy/manager.go`
   - Delete `reference/internal/proxy/storage.go`
   - Remove proxy manager initialization from API handlers

2. **Remove file storage:**
   - Remove `data/proxy_rules.json`
   - Remove storage-related configuration

3. **Update API handlers:**
   - Replace proxy manager calls with Kubernetes client calls
   - Update handlers to create/update ProxyRule CRDs instead

4. **Remove supervisord dependency:**
   - Remove Caddy from supervisord configuration
   - Remove supervisord-based reload logic

## Rollback Procedure

If issues occur during migration:

1. **Restore from backup:**
   ```bash
   # Restore JSON file from backup
   cp data/proxy_rules.json.backup data/proxy_rules.json
   ```

2. **Revert to old implementation:**
   - Restore old proxy manager code
   - Restart the old service
   - Remove ProxyRule CRDs if needed

3. **Scale down new operator:**
   ```bash
   kubectl scale deployment dns-operator --replicas=0
   ```

## Post-Migration Validation

After migration, verify:

- [ ] All proxy rules are present as CRDs
- [ ] ConfigMap contains correct Caddyfile
- [ ] Caddy is running and serving traffic
- [ ] Proxy rules are working correctly
- [ ] Controller is reconciling changes
- [ ] Status updates are working
- [ ] No errors in controller logs

## Troubleshooting

### Issue: ConfigMap not updating

**Solution:** Check controller logs for errors:
```bash
kubectl logs -l control-plane=controller-manager
```

### Issue: Caddy not reloading

**Solution:** 
- Verify ConfigMap is mounted correctly
- Check Caddy admin API is accessible
- Verify file watching is enabled in Caddy

### Issue: Proxy rules not working

**Solution:**
- Verify DNS records point to Caddy service
- Check Caddy logs for errors
- Verify target services are accessible
- Check network policies allow traffic

## Additional Considerations

### Webhook Validation

Consider adding a validating webhook for additional validation:

```go
// Validate hostname uniqueness
// Validate target IP accessibility
// Validate port availability
```

### Monitoring

Add metrics for:
- Number of active proxy rules
- ConfigMap update frequency
- Caddy reload success/failure rate
- Proxy request metrics

### Security

- Use NetworkPolicies to restrict Caddy access
- Use RBAC to limit who can create ProxyRules
- Consider admission webhooks for policy enforcement

## Summary

This migration guide covers:

1. ✅ CRD definition and installation
2. ✅ Controller implementation
3. ✅ Caddy Deployment setup
4. ✅ Data migration from JSON to CRDs
5. ✅ Testing strategies
6. ✅ Rollback procedures
7. ✅ Post-migration validation

The migration transforms the proxy domain from a file-based, imperative system to a Kubernetes-native, declarative operator pattern while maintaining all existing functionality.

