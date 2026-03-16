# Certificate Domain Migration Guide

## Overview

This guide provides step-by-step instructions for migrating the Certificate domain from the reference implementation (file-based, manager pattern) to a Kubernetes operator pattern using CRDs, controllers, and Kubernetes Secrets.

**Migration Scope:**
- File-based certificate storage → Kubernetes Secrets
- JSON domain storage → Certificate CRD spec/status
- Background renewal loops → Controller reconciliation
- ACME user registration files → Kubernetes Secrets
- Manager-based lifecycle → Controller-based reconciliation

## Prerequisites

- Kubernetes cluster (v1.24+)
- kubebuilder installed
- controller-runtime v0.15+
- Access to Cloudflare API token
- Let's Encrypt ACME account

## Migration Steps

### Step 1: Define Certificate CRD

Create the Certificate CRD schema using kubebuilder markers:

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Base Domain",type=string,JSONPath=`.spec.baseDomain`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Expires",type=date,JSONPath=`.status.expiresAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type Certificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	
	Spec   CertificateSpec   `json:"spec,omitempty"`
	Status CertificateStatus `json:"status,omitempty"`
}

type CertificateSpec struct {
	// Base domain for the certificate
	BaseDomain string `json:"baseDomain"`
	
	// ACME provider configuration
	Provider string `json:"provider"` // "letsencrypt" or "letsencrypt-staging"
	
	// Challenge type (only DNS-01 supported)
	ChallengeType string `json:"challengeType"` // "dns01"
	
	// Cloudflare configuration
	Cloudflare CloudflareConfig `json:"cloudflare"`
	
	// Subject Alternative Names (automatically managed)
	SANDomains []string `json:"sanDomains,omitempty"`
	
	// Auto-renewal configuration
	AutoRenew bool `json:"autoRenew,omitempty"`
	
	// Renewal threshold (days before expiration)
	RenewalThreshold int `json:"renewalThreshold,omitempty"` // default: 30
}

type CloudflareConfig struct {
	APITokenSecretRef SecretKeySelector `json:"apiTokenSecretRef"`
	ZoneID            string            `json:"zoneID,omitempty"` // auto-discovered if empty
}

type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type CertificateStatus struct {
	// Current state: Pending, Ready, Renewing, Error
	State string `json:"state,omitempty"`
	
	// Secret reference for certificate storage
	CertificateSecret *SecretReference `json:"certificateSecret,omitempty"`
	
	// Certificate expiration time
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
	
	// Number of SAN domains
	SANCount int `json:"sanCount,omitempty"`
	
	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	
	// Last renewal attempt
	LastRenewalAttempt *metav1.Time `json:"lastRenewalAttempt,omitempty"`
	
	// Error message if state is Error
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type SecretReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
```

**Key Changes:**
- Base domain and SAN domains moved to CRD spec
- Cloudflare token stored in Secret reference (not file/env)
- Certificate state tracked in status subresource
- Automatic SAN management via controller logic

### Step 2: Create Certificate Controller

Replace the manager pattern with a controller:

```go
// +kubebuilder:rbac:groups=certificate.jerkytreats.dev,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certificate.jerkytreats.dev,resources=certificates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=certificate.jerkytreats.dev,resources=certificates/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dns.jerkytreats.dev,resources=dnsrecords,verbs=get;list;watch

type CertificateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	
	// ACME client factory
	acmeClientFactory ACMEClientFactory
	
	// DNS provider factory
	dnsProviderFactory DNSProviderFactory
}

func (r *CertificateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cert certificatev1alpha1.Certificate
	if err := r.Get(ctx, req.NamespacedName, &cert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	
	// Handle deletion
	if !cert.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &cert)
	}
	
	// Reconcile certificate
	return r.reconcileCertificate(ctx, &cert)
}

func (r *CertificateReconciler) reconcileCertificate(ctx context.Context, cert *certificatev1alpha1.Certificate) (ctrl.Result, error) {
	// 1. Validate spec
	if err := r.validateSpec(cert); err != nil {
		return r.updateStatusError(ctx, cert, err)
	}
	
	// 2. Get Cloudflare token from Secret
	cfToken, err := r.getCloudflareToken(ctx, cert)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}
	
	// 3. Check if certificate needs renewal
	if r.needsRenewal(cert) {
		return r.renewCertificate(ctx, cert, cfToken)
	}
	
	// 4. Check if certificate exists
	secret, err := r.getCertificateSecret(ctx, cert)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	
	if secret == nil {
		// Certificate doesn't exist, obtain it
		return r.obtainCertificate(ctx, cert, cfToken)
	}
	
	// 5. Validate SAN domains against DNSRecords
	if err := r.validateSANDomains(ctx, cert); err != nil {
		return r.updateStatusError(ctx, cert, err)
	}
	
	// 6. Update status to Ready
	return r.updateStatusReady(ctx, cert, secret)
}
```

**Key Changes:**
- Manager methods → Controller reconcile function
- Background goroutines → RequeueAfter for scheduling
- File I/O → Kubernetes API calls
- Direct function calls → Event-driven reconciliation

### Step 3: Migrate Certificate Storage

**Before (File-based):**
```go
// reference/internal/certificate/manager.go
certPath := "/etc/letsencrypt/live/internal.example.test/fullchain.pem"
keyPath := "/etc/letsencrypt/live/internal.example.test/privkey.pem"

// Save certificate files
os.WriteFile(certPath, cert.Certificate, 0644)
os.WriteFile(keyPath, cert.PrivateKey, 0600)
```

**After (Kubernetes Secret):**
```go
// Store certificate in Secret
secret := &corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      fmt.Sprintf("%s-tls", cert.Name),
		Namespace: cert.Namespace,
		Labels: map[string]string{
			"certificate.jerkytreats.dev/certificate": cert.Name,
		},
	},
	Type: corev1.SecretTypeTLS,
	Data: map[string][]byte{
		corev1.TLSCertKey:       cert.Certificate,
		corev1.TLSPrivateKeyKey: cert.PrivateKey,
		"fullchain.pem":         cert.Certificate,
		"cert.pem":              cert.Certificate,
		"privkey.pem":           cert.PrivateKey,
	},
}

if err := r.Create(ctx, secret); err != nil {
	if !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, err
	}
	// Update existing secret
	if err := r.Update(ctx, secret); err != nil {
		return ctrl.Result{}, err
	}
}

// Update status with secret reference
cert.Status.CertificateSecret = &certificatev1alpha1.SecretReference{
	Name:      secret.Name,
	Namespace: secret.Namespace,
}
```

**Migration Path:**
1. Create Secret with certificate data
2. Update Certificate status with Secret reference
3. Mount Secret in CoreDNS pods (separate migration)
4. Remove file-based storage code

### Step 4: Migrate Domain Storage

**Before (JSON file):**
```go
// reference/internal/certificate/domain_storage.go
type CertificateDomains struct {
	BaseDomain string   `json:"base_domain"`
	SANDomains []string `json:"san_domains"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Stored in: data/certificate_domains.json
```

**After (CRD spec/status):**
```go
// Certificate CRD spec contains:
cert.Spec.BaseDomain = "internal.example.test"
cert.Spec.SANDomains = []string{
	"app.internal.example.test",
	"api.internal.example.test",
}

// Status tracks:
cert.Status.SANCount = len(cert.Spec.SANDomains)
```

**Migration Script:**
```go
// Migration helper to convert JSON to CRD
func migrateDomainStorage(jsonPath string) (*certificatev1alpha1.Certificate, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	
	var domains CertificateDomains
	if err := json.Unmarshal(data, &domains); err != nil {
		return nil, err
	}
	
	cert := &certificatev1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-cert", sanitizeDomain(domains.BaseDomain)),
		},
		Spec: certificatev1alpha1.CertificateSpec{
			BaseDomain: domains.BaseDomain,
			SANDomains: domains.SANDomains,
			Provider:   "letsencrypt",
			ChallengeType: "dns01",
			AutoRenew: true,
			RenewalThreshold: 30,
		},
	}
	
	return cert, nil
}
```

### Step 5: Migrate ACME User Registration

**Before (JSON file):**
```go
// reference/internal/certificate/manager.go
acmeUserPath := "/etc/letsencrypt/acme_user.json"
acmeKeyPath := "/etc/letsencrypt/acme_key.pem"

user, err := loadUser(acmeUserPath, acmeKeyPath)
```

**After (Kubernetes Secret):**
```go
// Store ACME user in Secret
const ACMEUserSecretName = "acme-user-registration"

func (r *CertificateReconciler) getOrCreateACMEUser(ctx context.Context, email string) (*User, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      ACMEUserSecretName,
		Namespace: "dns-operator-system", // operator namespace
	}
	
	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Create new ACME user
			user, err := r.createACMEUser(email)
			if err != nil {
				return nil, err
			}
			
			// Store in Secret
			userJSON, _ := json.Marshal(user.Registration)
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ACMEUserSecretName,
					Namespace: "dns-operator-system",
				},
				Data: map[string][]byte{
					"registration.json": userJSON,
					"private_key.pem":   encodePrivateKey(user.key),
				},
			}
			
			if err := r.Create(ctx, secret); err != nil {
				return nil, err
			}
			
			return user, nil
		}
		return nil, err
	}
	
	// Load from Secret
	return loadUserFromSecret(secret)
}
```

**Migration:**
1. Read existing ACME user files
2. Create Secret with user registration data
3. Update controller to use Secret
4. Remove file-based user loading

### Step 6: Migrate Renewal Loop

**Before (Background goroutine):**
```go
// reference/internal/certificate/manager.go
func (m *Manager) StartRenewalLoop(domain string) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				if m.shouldRenew(domain) {
					m.renewCertificate(domain)
				}
			case <-m.stopChan:
				return
			}
		}
	}()
}
```

**After (Controller RequeueAfter):**
```go
func (r *CertificateReconciler) reconcileCertificate(ctx context.Context, cert *certificatev1alpha1.Certificate) (ctrl.Result, error) {
	// ... existing reconciliation logic ...
	
	// Check if renewal is needed
	if r.needsRenewal(cert) {
		if cert.Status.State != "Renewing" {
			// Start renewal
			return r.renewCertificate(ctx, cert, cfToken)
		}
		// Already renewing, check status
		return r.checkRenewalStatus(ctx, cert)
	}
	
	// Calculate next renewal check time
	expiresAt := cert.Status.ExpiresAt
	if expiresAt != nil {
		renewalThreshold := time.Duration(cert.Spec.RenewalThreshold) * 24 * time.Hour
		nextCheck := expiresAt.Time.Add(-renewalThreshold)
		now := time.Now()
		
		if nextCheck.After(now) {
			// Schedule next check
			return ctrl.Result{
				RequeueAfter: nextCheck.Sub(now),
			}, nil
		}
	}
	
	// Default: check daily
	return ctrl.Result{
		RequeueAfter: 24 * time.Hour,
	}, nil
}

func (r *CertificateReconciler) needsRenewal(cert *certificatev1alpha1.Certificate) bool {
	if cert.Status.ExpiresAt == nil {
		return false
	}
	
	renewalThreshold := time.Duration(cert.Spec.RenewalThreshold) * 24 * time.Hour
	renewalTime := cert.Status.ExpiresAt.Time.Add(-renewalThreshold)
	
	return time.Now().After(renewalTime)
}
```

**Key Changes:**
- Background goroutines → RequeueAfter scheduling
- Continuous polling → Event-driven checks
- Manual stop channels → Controller lifecycle management

### Step 7: Implement Automatic SAN Management

**Before (Manual SAN management):**
```go
// reference/internal/certificate/manager.go
func (m *Manager) AddDomainToSAN(domain string) error {
	// Manual addition via API call
}
```

**After (Automatic via DNSRecord watching):**
```go
// Watch DNSRecords and automatically update Certificate SAN list
func (r *CertificateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Watch Certificates
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&certificatev1alpha1.Certificate{}).
		Complete(r); err != nil {
		return err
	}
	
	// Watch DNSRecords for automatic SAN management
	return ctrl.NewControllerManagedBy(mgr).
		For(&certificatev1alpha1.Certificate{}).
		Watches(
			&source.Kind{Type: &dnsv1alpha1.DNSRecord{}},
			handler.EnqueueRequestsFromMapFunc(r.mapDNSRecordToCertificates),
		).
		Complete(r)
}

func (r *CertificateReconciler) mapDNSRecordToCertificates(obj client.Object) []reconcile.Request {
	dnsRecord := obj.(*dnsv1alpha1.DNSRecord)
	
	// Find all Certificates that should include this DNS record
	var certs certificatev1alpha1.CertificateList
	if err := r.List(context.Background(), &certs); err != nil {
		return []reconcile.Request{}
	}
	
	var requests []reconcile.Request
	for _, cert := range certs.Items {
		// Check if DNS record FQDN should be in certificate SAN list
		if r.shouldIncludeInSAN(&cert, dnsRecord.Spec.FQDN) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cert.Name,
					Namespace: cert.Namespace,
				},
			})
		}
	}
	
	return requests
}

func (r *CertificateReconciler) validateSANDomains(ctx context.Context, cert *certificatev1alpha1.Certificate) error {
	// Get all DNSRecords
	var dnsRecords dnsv1alpha1.DNSRecordList
	if err := r.List(ctx, &dnsRecords); err != nil {
		return err
	}
	
	// Build expected SAN list from DNSRecords
	expectedSANs := []string{}
	for _, record := range dnsRecords.Items {
		if r.shouldIncludeInSAN(cert, record.Spec.FQDN) {
			expectedSANs = append(expectedSANs, record.Spec.FQDN)
		}
	}
	
	// Compare with current SAN list
	if !equalStringSlices(cert.Spec.SANDomains, expectedSANs) {
		// Update SAN list
		cert.Spec.SANDomains = expectedSANs
		if err := r.Update(ctx, cert); err != nil {
			return err
		}
		
		// Trigger certificate renewal
		return r.renewCertificate(ctx, cert, cfToken)
	}
	
	return nil
}
```

**Key Changes:**
- Manual API calls → Automatic controller watching
- Explicit SAN management → Implicit via DNSRecord references
- Certificate renewal triggered automatically on SAN changes

### Step 8: Preserve DNS Provider Cleanup

**Before (CleaningDNSProvider wrapper):**
```go
// reference/internal/certificate/provider.go
type CleaningDNSProvider struct {
	provider  challenge.Provider
	token     string
	zoneID    string
}

func (p *CleaningDNSProvider) Present(domain, token, keyAuth string) error {
	// Present challenge
	// Proactive cleanup on error
}
```

**After (Controller cleanup logic):**
```go
func (r *CertificateReconciler) obtainCertificate(ctx context.Context, cert *certificatev1alpha1.Certificate, cfToken string) (ctrl.Result, error) {
	// Create DNS provider
	dnsProvider, err := r.dnsProviderFactory.Create(cfToken, cert.Spec.Cloudflare.ZoneID)
	if err != nil {
		return ctrl.Result{}, err
	}
	
	// Wrap with cleanup tracking
	cleanupTracker := &challengeCleanupTracker{
		provider: dnsProvider,
		zoneID:   cert.Spec.Cloudflare.ZoneID,
		token:    cfToken,
	}
	
	defer func() {
		// Ensure cleanup on error
		if err != nil {
			cleanupTracker.cleanupAll()
		}
	}()
	
	// Obtain certificate via ACME
	acmeCert, err := r.acmeClient.ObtainCertificate(domains, cleanupTracker)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}
	
	// Cleanup challenge records
	if err := cleanupTracker.cleanupAll(); err != nil {
		log.Error(err, "Failed to cleanup challenge records")
		// Non-fatal, continue
	}
	
	// Store certificate in Secret
	return r.storeCertificate(ctx, cert, acmeCert)
}
```

**Key Changes:**
- Provider wrapper → Controller-level cleanup
- Automatic cleanup → Explicit cleanup in controller
- Error handling preserved

### Step 9: Handle Rate Limiting

**Before (Exponential backoff in renewal):**
```go
// reference/internal/certificate/backoff.go
func exponentialBackoff(attempt int) time.Duration {
	return time.Duration(math.Pow(2, float64(attempt))) * time.Minute
}
```

**After (RequeueAfter with backoff):**
```go
func (r *CertificateReconciler) handleRateLimitError(ctx context.Context, cert *certificatev1alpha1.Certificate, err error) (ctrl.Result, error) {
	// Detect rate limit error
	if !isRateLimitError(err) {
		return ctrl.Result{}, err
	}
	
	// Update status
	cert.Status.State = "Error"
	cert.Status.ErrorMessage = "Rate limited by Let's Encrypt"
	cert.Status.Conditions = append(cert.Status.Conditions, metav1.Condition{
		Type:    "RateLimited",
		Status:  "True",
		Reason:  "LetEncryptRateLimit",
		Message: err.Error(),
	})
	
	if err := r.Status().Update(ctx, cert); err != nil {
		return ctrl.Result{}, err
	}
	
	// Exponential backoff: 1h, 2h, 4h, 8h, max 24h
	attempt := getRateLimitAttempt(cert)
	backoff := time.Duration(math.Min(math.Pow(2, float64(attempt)), 24)) * time.Hour
	
	return ctrl.Result{
		RequeueAfter: backoff,
	}, nil
}
```

**Key Changes:**
- Function-level backoff → Controller RequeueAfter
- Rate limit state tracked in Certificate status
- Exponential backoff preserved

### Step 10: Update CoreDNS Integration

**Before (Direct file path):**
```go
// reference/internal/certificate/manager.go
m.corednsConfigManager.EnableTLS(domain, certPath, keyPath)
```

**After (Secret mount):**
```go
// CoreDNS ConfigMap update (separate controller)
func (r *CoreDNSReconciler) updateTLSConfig(ctx context.Context, cert *certificatev1alpha1.Certificate) error {
	// Get certificate secret
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cert.Status.CertificateSecret.Name,
		Namespace: cert.Status.CertificateSecret.Namespace,
	}, secret); err != nil {
		return err
	}
	
	// Update CoreDNS ConfigMap with TLS configuration
	// Reference secret mount path
	corefile := buildCorefileWithTLS(cert.Spec.BaseDomain, secret)
	
	// Update ConfigMap
	cm := &corev1.ConfigMap{}
	// ... update logic ...
	
	return r.Update(ctx, cm)
}
```

**Migration:**
1. CoreDNS pods mount certificate Secrets
2. CoreDNS ConfigMap references secret paths
3. Remove direct file path configuration

## Testing Strategy

### Unit Tests

```go
func TestCertificateReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name    string
		cert    *certificatev1alpha1.Certificate
		wantErr bool
	}{
		{
			name: "obtain new certificate",
			cert: &certificatev1alpha1.Certificate{
				Spec: certificatev1alpha1.CertificateSpec{
					BaseDomain: "test.example.com",
					Provider:   "letsencrypt-staging",
				},
			},
		},
		{
			name: "renew expiring certificate",
			cert: &certificatev1alpha1.Certificate{
				Spec: certificatev1alpha1.CertificateSpec{
					BaseDomain: "test.example.com",
				},
				Status: certificatev1alpha1.CertificateStatus{
					ExpiresAt: &metav1.Time{Time: time.Now().Add(20 * 24 * time.Hour)},
				},
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client
			// Run reconciliation
			// Assert results
		})
	}
}
```

### Integration Tests

```go
func TestCertificate_EndToEnd(t *testing.T) {
	// 1. Create Certificate CRD
	// 2. Create Cloudflare token Secret
	// 3. Wait for certificate issuance
	// 4. Verify Secret created
	// 5. Verify status updated
	// 6. Create DNSRecord
	// 7. Verify SAN list updated
	// 8. Verify certificate renewed
}
```

## Migration Checklist

- [ ] Define Certificate CRD with kubebuilder
- [ ] Implement Certificate controller
- [ ] Migrate certificate storage to Secrets
- [ ] Migrate domain storage to CRD spec
- [ ] Migrate ACME user to Secret
- [ ] Replace renewal loop with RequeueAfter
- [ ] Implement automatic SAN management
- [ ] Preserve DNS provider cleanup logic
- [ ] Handle rate limiting with backoff
- [ ] Update CoreDNS integration
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Update documentation
- [ ] Create migration script for existing certificates

## Rollback Plan

If migration issues occur:

1. **Keep reference implementation running** during migration
2. **Dual-write certificates** to both file system and Secrets initially
3. **Gradual cutover** - migrate one certificate at a time
4. **Monitor certificate expiration** - ensure renewals work
5. **Rollback procedure:**
   - Revert Certificate CRD
   - Restore file-based storage
   - Re-enable reference implementation

## Common Pitfalls

1. **Secret permissions**: Ensure controller has RBAC to create/update Secrets
2. **Namespace isolation**: Certificate Secrets must be in accessible namespace
3. **Rate limiting**: Let's Encrypt has strict rate limits - use staging for testing
4. **DNS propagation**: DNS-01 challenges require DNS propagation time
5. **Certificate expiration**: Monitor expiration dates during migration
6. **SAN validation**: Ensure DNSRecord watching works correctly
7. **Cleanup failures**: Challenge record cleanup failures should not block issuance

## Next Steps

After completing this migration:

1. Migrate CoreDNS integration to use Secrets
2. Implement Certificate validating webhook
3. Add Certificate metrics and observability
4. Document Certificate CRD usage
5. Create Certificate examples and tutorials
