# Migration Guide: API Server Command to Kubernetes Operator

## Overview

This guide provides step-by-step instructions for migrating the API Server command (`reference/cmd/api/main.go`) from a standalone HTTP server application to a Kubernetes operator using controller-runtime.

## Migration Goals

- Replace HTTP server with controller-runtime manager
- Convert REST API endpoints to Kubernetes CRD controllers
- Replace background processes with controller reconciliation loops
- Migrate configuration from config files to ConfigMaps
- Replace HTTP health checks with Kubernetes probes
- Maintain all existing functionality through Kubernetes-native patterns

## Prerequisites

- Kubernetes cluster (v1.20+)
- kubebuilder CLI installed
- controller-runtime v0.15.0+ understanding
- Go 1.19+ development environment

## Step-by-Step Migration

### Step 1: Initialize Operator Project Structure

**Current State:** Single binary with HTTP server

**Target State:** Kubernetes operator with controller-runtime manager

**Actions:**

1. Initialize kubebuilder project (if not already done):
```bash
kubebuilder init --domain dns-operator.io --repo github.com/jerkytreats/dns-operator
```

2. Create the operator main entry point structure:
```go
// cmd/operator/main.go
package main

import (
    "flag"
    "os"
    
    "k8s.io/apimachinery/pkg/runtime"
    utilruntime "k8s.io/apimachinery/pkg/util/runtime"
    clientgoscheme "k8s.io/client-go/kubernetes/scheme"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/healthz"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    
    dnsv1alpha1 "github.com/jerkytreats/dns-operator/api/v1alpha1"
    "github.com/jerkytreats/dns-operator/internal/controller"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    utilruntime.Must(dnsv1alpha1.AddToScheme(scheme))
}
```

### Step 2: Replace HTTP Server with Controller Manager

**Current Code Pattern:**
```go
// reference/cmd/api/main.go
server := &http.Server{
    Addr:         fmt.Sprintf("%s:%d", host, port),
    ReadTimeout:  readTimeout,
    WriteTimeout: writeTimeout,
    IdleTimeout:  idleTimeout,
    Handler:      mux,
}
server.ListenAndServe()
```

**Migration:**

Replace HTTP server initialization with controller-runtime manager:

```go
// cmd/operator/main.go
func main() {
    var metricsAddr string
    var enableLeaderElection bool
    var probeAddr string
    flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
    flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
    flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
    flag.Parse()

    ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:                 scheme,
        Metrics:                metricsserver.Options{BindAddress: metricsAddr},
        HealthProbeBindAddress: probeAddr,
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "dns-operator-leader-election",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    // Setup health checks
    if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up health check")
        os.Exit(1)
    }
    if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up ready check")
        os.Exit(1)
    }

    // Initialize controllers (see Step 3)
    // ...

    setupLog.Info("starting manager")
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
```

**Key Changes:**
- Remove HTTP server setup
- Use `ctrl.NewManager()` instead
- Replace HTTP health endpoints with `AddHealthzCheck()` and `AddReadyzCheck()`
- Use `mgr.Start()` for lifecycle management
- Graceful shutdown handled by `ctrl.SetupSignalHandler()`

### Step 3: Convert Component Initialization to Controller Setup

**Current Pattern:**
```go
// reference/cmd/api/main.go
tailscaleClient, _ := tailscale.NewClient()
firewallManager, _ := firewall.NewManager()
dnsManager := newCoreDNSManager(currentDeviceIP)
proxyManager, _ := proxy.NewManager(nil)
certificateManager := certProcess.GetManager()
handlerRegistry, _ := handler.NewHandlerRegistry(...)
```

**Migration:**

Initialize components within controller setup functions:

```go
// cmd/operator/main.go
func main() {
    // ... manager setup ...
    
    // Get Kubernetes client
    k8sClient := mgr.GetClient()
    
    // Initialize shared components
    tailscaleClient, err := tailscale.NewClient()
    if err != nil {
        setupLog.Error(err, "unable to create Tailscale client")
        os.Exit(1)
    }
    
    firewallManager, err := firewall.NewManager()
    if err != nil {
        setupLog.Error(err, "unable to create firewall manager")
        os.Exit(1)
    }
    
    // Setup DNS Record Controller
    if err = (&controller.DNSRecordReconciler{
        Client:          k8sClient,
        Scheme:          mgr.GetScheme(),
        TailscaleClient: tailscaleClient,
        FirewallManager: firewallManager,
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "DNSRecord")
        os.Exit(1)
    }
    
    // Setup Certificate Controller
    if err = (&controller.CertificateReconciler{
        Client: k8sClient,
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "Certificate")
        os.Exit(1)
    }
    
    // Setup Device Sync Controller
    if err = (&controller.DeviceSyncReconciler{
        Client:          k8sClient,
        Scheme:          mgr.GetScheme(),
        TailscaleClient: tailscaleClient,
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "DeviceSync")
        os.Exit(1)
    }
    
    // ... start manager ...
}
```

**Key Changes:**
- Components initialized once and passed to controllers
- Controllers receive dependencies via struct fields
- Use `SetupWithManager()` pattern for controller registration
- Components shared across controllers via dependency injection

### Step 4: Replace REST API Handlers with CRD Controllers

**Current Pattern:**
```go
// reference/cmd/api/main.go
handlerRegistry.RegisterHandlers(mux)
// Handles: GET /records, POST /add-record, DELETE /remove-record, etc.
```

**Migration:**

Create controllers for each resource type:

```go
// internal/controller/dnsrecord_controller.go
type DNSRecordReconciler struct {
    client.Client
    Scheme          *runtime.Scheme
    TailscaleClient *tailscale.Client
    FirewallManager *firewall.Manager
    DNSManager      *coredns.Manager
    ProxyManager    *proxy.Manager
}

func (r *DNSRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.Log.FromContext(ctx)
    
    // Fetch DNSRecord CRD
    var dnsRecord dnsv1alpha1.DNSRecord
    if err := r.Get(ctx, req.NamespacedName, &dnsRecord); err != nil {
        if apierrors.IsNotFound(err) {
            // Resource deleted - handle cleanup
            return r.handleDeletion(ctx, req.NamespacedName)
        }
        return ctrl.Result{}, err
    }
    
    // Reconcile DNS record
    if err := r.reconcileDNSRecord(ctx, &dnsRecord); err != nil {
        log.Error(err, "failed to reconcile DNS record")
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}

func (r *DNSRecordReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&dnsv1alpha1.DNSRecord{}).
        Complete(r)
}
```

**API Endpoint Mapping:**

| Current REST Endpoint | Kubernetes Resource | Controller Method |
|----------------------|---------------------|-------------------|
| `POST /add-record` | `DNSRecord` CRD create | `Reconcile()` on create event |
| `GET /records` | `kubectl get dnsrecords` | Kubernetes API (no controller needed) |
| `DELETE /remove-record` | `DNSRecord` CRD delete | `Reconcile()` on delete event |
| `POST /update-record` | `DNSRecord` CRD update | `Reconcile()` on update event |

### Step 5: Replace Background Processes with Reconciliation Loops

**Current Pattern:**
```go
// reference/cmd/api/main.go
// Certificate process
certProcess.StartWithRetry(30 * time.Second)

// Sync manager polling
syncManager.StartPolling(syncConfig.Polling.Interval)

// DNS API self-registration
go registerDNSAPIService()
```

**Migration:**

Use controller reconciliation with `RequeueAfter` for periodic tasks:

```go
// internal/controller/devicesync_controller.go
func (r *DeviceSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.Log.FromContext(ctx)
    
    // Fetch DeviceSync CRD
    var deviceSync dnsv1alpha1.DeviceSync
    if err := r.Get(ctx, req.NamespacedName, &deviceSync); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Perform sync operation
    if err := r.syncDevices(ctx, &deviceSync); err != nil {
        log.Error(err, "failed to sync devices")
        return ctrl.Result{}, err
    }
    
    // Requeue after polling interval
    requeueAfter := time.Duration(deviceSync.Spec.PollingIntervalSeconds) * time.Second
    return ctrl.Result{RequeueAfter: requeueAfter}, nil
}
```

**Background Process Mapping:**

| Current Background Process | Kubernetes Pattern | Implementation |
|---------------------------|-------------------|----------------|
| Certificate process | Certificate Controller | Reconcile with `RequeueAfter` |
| Device sync polling | DeviceSync Controller | Reconcile with `RequeueAfter` |
| DNS API self-registration | Init container or Job | Kubernetes Job or init container |
| Certificate readiness waiting | Controller watch | Watch Certificate CRD status |

### Step 6: Migrate Configuration to ConfigMap

**Current Pattern:**
```go
// reference/cmd/api/main.go
config.FirstTimeInit(configFile)
config.CheckRequiredKeys()
host := config.GetString(ServerHostKey)
port := config.GetInt(ServerPortKey)
```

**Migration:**

Use ConfigMap for operator configuration:

1. Create ConfigMap manifest:
```yaml
# config/operator-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dns-operator-config
  namespace: dns-operator-system
data:
  config.yaml: |
    server:
      host: "0.0.0.0"
      port: 8080
    dns:
      domain: "example.local"
    tailscale:
      device_name: "dns-operator"
```

2. Load ConfigMap in operator:
```go
// internal/config/operator_config.go
type OperatorConfig struct {
    Server    ServerConfig    `yaml:"server"`
    DNS       DNSConfig       `yaml:"dns"`
    Tailscale TailscaleConfig `yaml:"tailscale"`
}

func LoadFromConfigMap(ctx context.Context, client client.Client, namespace, name string) (*OperatorConfig, error) {
    var cm corev1.ConfigMap
    key := types.NamespacedName{Namespace: namespace, Name: name}
    if err := client.Get(ctx, key, &cm); err != nil {
        return nil, err
    }
    
    config := &OperatorConfig{}
    if err := yaml.Unmarshal([]byte(cm.Data["config.yaml"]), config); err != nil {
        return nil, err
    }
    
    return config, nil
}
```

3. Use in controller:
```go
// internal/controller/dnsrecord_controller.go
func (r *DNSRecordReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Load config from ConfigMap
    config, err := config.LoadFromConfigMap(ctx, r.Client, "dns-operator-system", "dns-operator-config")
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // Use config values
    baseDomain := config.DNS.Domain
    // ...
}
```

**Configuration Key Migration:**

| Current Config Key | ConfigMap Path | Environment Variable Alternative |
|-------------------|----------------|----------------------------------|
| `server.host` | `config.yaml.server.host` | `SERVER_HOST` |
| `server.port` | `config.yaml.server.port` | `SERVER_PORT` |
| `dns.domain` | `config.yaml.dns.domain` | `DNS_DOMAIN` |
| `tailscale.device_name` | `config.yaml.tailscale.device_name` | `TAILSCALE_DEVICE_NAME` |

### Step 7: Replace HTTP Health Checks with Kubernetes Probes

**Current Pattern:**
```go
// reference/cmd/api/main.go
// HTTP health check endpoint via handler
// GET /health
```

**Migration:**

Use Kubernetes liveness and readiness probes:

1. In operator main:
```go
// cmd/operator/main.go
if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
    setupLog.Error(err, "unable to set up health check")
    os.Exit(1)
}

if err := mgr.AddReadyzCheck("readyz", func(req *http.Request) error {
    // Custom readiness check
    if !r.DNSManager.IsReady() {
        return fmt.Errorf("DNS manager not ready")
    }
    return nil
}); err != nil {
    setupLog.Error(err, "unable to set up ready check")
    os.Exit(1)
}
```

2. In Deployment manifest:
```yaml
# config/manager/manager.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dns-operator-controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
```

**Health Check Mapping:**

| Current Health Check | Kubernetes Pattern | Implementation |
|---------------------|-------------------|----------------|
| HTTP `/health` endpoint | Liveness probe | `AddHealthzCheck()` |
| HTTP `/ready` endpoint | Readiness probe | `AddReadyzCheck()` |
| DNS health check | Custom probe | Custom `readyz` check function |
| Component status | CRD status field | Update `.status.conditions` |

### Step 8: Handle Graceful Shutdown

**Current Pattern:**
```go
// reference/cmd/api/main.go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit
server.Shutdown(ctx)
```

**Migration:**

Controller-runtime handles graceful shutdown automatically:

```go
// cmd/operator/main.go
// No manual signal handling needed
if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
    setupLog.Error(err, "problem running manager")
    os.Exit(1)
}
```

**Key Changes:**
- `ctrl.SetupSignalHandler()` handles SIGINT/SIGTERM
- Manager automatically shuts down controllers gracefully
- Context cancellation propagates to all controllers
- No manual shutdown coordination needed

### Step 9: Remove HTTP Server Dependencies

**Actions:**

1. Remove HTTP server imports:
```go
// Remove these imports:
// "net/http"
// "net"
// "crypto/tls"
```

2. Remove handler registry:
```go
// Remove:
// handlerRegistry, _ := handler.NewHandlerRegistry(...)
// handlerRegistry.RegisterHandlers(mux)
```

3. Keep only metrics/health endpoints:
```go
// Keep only for metrics and health probes:
// Metrics server (via manager options)
// Health/ready probes (via AddHealthzCheck/AddReadyzCheck)
```

### Step 10: Update Testing Strategy

**Current Testing:**
```go
// reference/cmd/api/main_test.go
// Integration tests with HTTP test server
```

**Migration:**

Use controller-runtime testing utilities:

```go
// internal/controller/dnsrecord_controller_test.go
func TestDNSRecordReconciler(t *testing.T) {
    testEnv := &envtest.Environment{
        CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
        ErrorIfCRDPathMissing: true,
    }
    
    cfg, err := testEnv.Start()
    // ... setup ...
    
    k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
    // ... setup client ...
    
    r := &DNSRecordReconciler{
        Client: k8sClient,
        Scheme: scheme,
        // ... other dependencies ...
    }
    
    // Test reconciliation
    result, err := r.Reconcile(ctx, ctrl.Request{
        NamespacedName: types.NamespacedName{
            Name:      "test-record",
            Namespace: "default",
        },
    })
    // ... assertions ...
}
```

## Migration Checklist

- [ ] Initialize kubebuilder project structure
- [ ] Replace HTTP server with controller-runtime manager
- [ ] Convert component initialization to controller setup
- [ ] Create CRD definitions for DNSRecord, Certificate, DeviceSync
- [ ] Implement controllers for each resource type
- [ ] Replace REST API handlers with controller Reconcile methods
- [ ] Convert background processes to reconciliation loops with RequeueAfter
- [ ] Migrate configuration from config file to ConfigMap
- [ ] Replace HTTP health checks with Kubernetes probes
- [ ] Update graceful shutdown to use controller-runtime patterns
- [ ] Remove HTTP server dependencies
- [ ] Update tests to use controller-runtime testing utilities
- [ ] Create Deployment manifest for operator
- [ ] Create ConfigMap manifest for operator configuration
- [ ] Update documentation for Kubernetes-native usage

## Post-Migration Validation

1. **Deploy operator:**
```bash
make deploy
```

2. **Verify operator is running:**
```bash
kubectl get pods -n dns-operator-system
```

3. **Test CRD creation:**
```bash
kubectl apply -f config/samples/dns_v1alpha1_dnsrecord.yaml
```

4. **Verify reconciliation:**
```bash
kubectl get dnsrecord -o yaml
kubectl describe dnsrecord <name>
```

5. **Check operator logs:**
```bash
kubectl logs -n dns-operator-system deployment/dns-operator-controller-manager
```

## Common Pitfalls and Solutions

### Pitfall 1: Component Initialization Order

**Problem:** Components initialized in wrong order causing dependencies to fail.

**Solution:** Initialize all components before setting up controllers, pass as dependencies.

### Pitfall 2: Configuration Loading

**Problem:** ConfigMap not found or not loaded correctly.

**Solution:** Ensure ConfigMap exists before starting manager, handle missing ConfigMap gracefully.

### Pitfall 3: Background Process Timing

**Problem:** Periodic tasks not running at expected intervals.

**Solution:** Use `RequeueAfter` correctly, ensure reconciliation completes before requeue.

### Pitfall 4: Resource Cleanup

**Problem:** Resources not cleaned up on deletion.

**Solution:** Implement proper finalizers and cleanup logic in controller Reconcile method.

## Additional Resources

- [controller-runtime Documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [kubebuilder Book](https://book.kubebuilder.io/)
- [Kubernetes Operator Best Practices](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

## Summary

The migration from HTTP server to Kubernetes operator involves:

1. **Architecture Change:** HTTP server → controller-runtime manager
2. **API Change:** REST endpoints → CRD controllers
3. **Process Change:** Background goroutines → reconciliation loops
4. **Config Change:** Config files → ConfigMaps
5. **Health Change:** HTTP endpoints → Kubernetes probes
6. **Lifecycle Change:** Manual shutdown → controller-runtime lifecycle

All existing functionality is preserved through Kubernetes-native patterns, providing better integration with Kubernetes ecosystems and improved observability.

