# Phase 2: Resource Contracts and Status Model

## Goal

Define stable resource schemas before deeper controller work begins.

## Scope

- Create the first version of `DNSRecord`, `TailscaleDevice`, `Certificate`, and `ProxyRule`.
- Define shared condition names, observed generation handling, and status message conventions.
- Model references between resources with explicit typed references.
- Move secret material behind `Secret` references.

## Version Strategy

- Start with `v1alpha1` for all operator owned APIs.
- Keep wire formats simple and additive.
- Avoid speculative fields that do not serve the first delivery slices.
- Prefer nested structs over flat boolean sprawl.

## Shared API Conventions

### Metadata

- All resources are namespaced.
- `metadata.name` should be human meaningful and stable.
- Labels should support zone level grouping, runtime ownership, and migration provenance.
- Annotations should be reserved for operator internals, migration notes, and manual pause controls.

### Status

Each resource should expose:

- `observedGeneration`
- `conditions`
- a small set of resource specific status fields that help users understand the current effective state

### Common Condition Types

The following condition names should be reused where they fit:

- `Ready`
- `InputValid`
- `ReferencesResolved`
- `CredentialsReady`
- `Rendered`
- `RuntimeSynced`
- `Accepted`

### Reference Rules

- References should use explicit `name` and optional `namespace` fields.
- Cross namespace references should be opt in and documented per resource kind.
- Secret references should point to both secret name and key where needed.
- Controllers should report broken references in conditions rather than hiding them in logs only.

## Resource Design Priorities

### `DNSRecord`

- Hold the desired record name, zone, type, and target.
- Support either a direct target value or a `TailscaleDevice` reference.
- Carry optional proxy intent as a structured field block, not scattered booleans.
- Report resolved target, rendered zone owner, and readiness conditions in status.

**Proposed spec shape**

- `name` as the record label within the zone
- `zone` as the managed DNS zone
- `type` as `A`, `AAAA`, `CNAME`, or `TXT`
- `ttl` as an integer with a sane default
- `target.value` for direct literal targets
- `target.tailscaleDeviceRef` for device backed target resolution
- `proxy.enabled`, `proxy.targetPort`, and `proxy.protocol` for later proxy integration

**Proposed status shape**

- `observedGeneration`
- `fqdn`
- `resolvedTarget`
- `zoneConfigMapName`
- `conditions`

**Validation goals**

- enforce record type enum
- enforce valid zone and record label format
- require exactly one target source for the first version
- bound `ttl` to a sensible minimum

### `TailscaleDevice`

- Hold the desired device identity and polling policy.
- Reference API credentials through a `Secret`.
- Report current device id, current IP, sync time, and readiness conditions in status.

**Proposed spec shape**

- `hostname` as the lookup key in Tailscale
- `tailnet` as optional override when operator config is not global
- `auth.secretRef.name`
- `auth.secretRef.key`
- `syncInterval`
- `annotations` map for imported metadata that should remain visible

**Proposed status shape**

- `observedGeneration`
- `deviceID`
- `tailscaleIP`
- `online`
- `lastSyncedAt`
- `conditions`

**Validation goals**

- require hostname
- require secret reference fields when credentials are resource specific
- require `syncInterval` to stay within an allowed range

### `Certificate`

- Hold requested domains, issuer settings, renewal policy, and challenge strategy.
- Reference credential material through `Secret` resources.
- Report issued state, expiry, target secret name, and failure conditions in status.

**Proposed spec shape**

- `domains` as the requested subject set
- `issuer.provider` as `letsencrypt` or `letsencrypt-staging`
- `issuer.email`
- `challenge.type` as `dns01`
- `challenge.cloudflare.apiTokenSecretRef.name`
- `challenge.cloudflare.apiTokenSecretRef.key`
- `secretTemplate.name` as the target TLS secret name
- `autoRenew`
- `renewBefore`

**Proposed status shape**

- `observedGeneration`
- `state`
- `certificateSecretRef`
- `expiresAt`
- `lastIssuedAt`
- `conditions`

**Validation goals**

- require at least one domain
- require supported issuer and challenge values
- require secret refs for DNS provider credentials
- ensure target secret naming is deterministic

### `ProxyRule`

- Hold host match rules, backend target, transport settings, and certificate linkage.
- Report rendered config generation, runtime sync state, and readiness conditions in status.

**Proposed spec shape**

- `hostname`
- `backend.address`
- `backend.port`
- `backend.protocol`
- `tls.mode`
- `tls.certificateRef`
- `enabled`

**Proposed status shape**

- `observedGeneration`
- `state`
- `renderedConfigMapName`
- `lastAppliedHash`
- `conditions`

**Validation goals**

- require hostname and backend port
- restrict backend protocol enum
- keep TLS linkage explicit and optional

## Deliverables

- Generated CRD manifests with OpenAPI validation.
- Example resource manifests for each resource kind.
- Shared API conventions for labels, annotations, and status conditions.
- A short design note for cross resource ownership and reference rules.

## Concrete Type Sketches

The following shapes are the preferred baseline for implementation planning.

### `DNSRecord`

```go
type ObjectReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type SecretKeyReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key"`
}

type DNSRecordTarget struct {
	Value              string           `json:"value,omitempty"`
	TailscaleDeviceRef *ObjectReference `json:"tailscaleDeviceRef,omitempty"`
}

type DNSRecordProxy struct {
	Enabled    bool   `json:"enabled,omitempty"`
	TargetPort int32  `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

type DNSRecordSpec struct {
	Name   string          `json:"name"`
	Zone   string          `json:"zone"`
	Type   string          `json:"type,omitempty"`
	TTL    int32           `json:"ttl,omitempty"`
	Target DNSRecordTarget `json:"target"`
	Proxy  *DNSRecordProxy `json:"proxy,omitempty"`
}

type DNSRecordStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	FQDN               string             `json:"fqdn,omitempty"`
	ResolvedTarget     string             `json:"resolvedTarget,omitempty"`
	ZoneConfigMapName  string             `json:"zoneConfigMapName,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}
```

### `TailscaleDevice`

```go
type TailscaleAuth struct {
	APIKeySecretRef SecretKeyReference `json:"apiKeySecretRef"`
}

type TailscaleDeviceSpec struct {
	Hostname     string            `json:"hostname"`
	Tailnet      string            `json:"tailnet,omitempty"`
	Auth         TailscaleAuth     `json:"auth"`
	SyncInterval metav1.Duration   `json:"syncInterval,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
}

type TailscaleDeviceStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	DeviceID           string             `json:"deviceID,omitempty"`
	TailscaleIP        string             `json:"tailscaleIP,omitempty"`
	Online             bool               `json:"online,omitempty"`
	LastSyncedAt       *metav1.Time       `json:"lastSyncedAt,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}
```

### `Certificate`

```go
type CloudflareChallenge struct {
	APITokenSecretRef SecretKeyReference `json:"apiTokenSecretRef"`
}

type CertificateIssuer struct {
	Provider string `json:"provider"`
	Email    string `json:"email"`
}

type CertificateChallenge struct {
	Type       string               `json:"type"`
	Cloudflare CloudflareChallenge  `json:"cloudflare"`
}

type SecretTemplate struct {
	Name string `json:"name"`
}

type CertificateSpec struct {
	Domains        []string              `json:"domains"`
	Issuer         CertificateIssuer     `json:"issuer"`
	Challenge      CertificateChallenge  `json:"challenge"`
	SecretTemplate SecretTemplate        `json:"secretTemplate"`
	AutoRenew      bool                  `json:"autoRenew,omitempty"`
	RenewBefore    metav1.Duration       `json:"renewBefore,omitempty"`
}

type CertificateStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	State              string             `json:"state,omitempty"`
	CertificateSecret  *ObjectReference   `json:"certificateSecret,omitempty"`
	ExpiresAt          *metav1.Time       `json:"expiresAt,omitempty"`
	LastIssuedAt       *metav1.Time       `json:"lastIssuedAt,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}
```

### `ProxyRule`

```go
type ProxyBackend struct {
	Address  string `json:"address"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol,omitempty"`
}

type ProxyTLS struct {
	Mode           string           `json:"mode,omitempty"`
	CertificateRef *ObjectReference `json:"certificateRef,omitempty"`
}

type ProxyRuleSpec struct {
	Hostname string       `json:"hostname"`
	Backend  ProxyBackend `json:"backend"`
	TLS      *ProxyTLS    `json:"tls,omitempty"`
	Enabled  bool         `json:"enabled,omitempty"`
}

type ProxyRuleStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	State              string             `json:"state,omitempty"`
	RenderedConfigMap  string             `json:"renderedConfigMap,omitempty"`
	LastAppliedHash    string             `json:"lastAppliedHash,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}
```

## Sample Manifests

Concrete examples live in [Phase 2 sample manifests](phase-2-sample-manifests.md).

## Key Decisions

- No raw API keys or tokens in any spec field.
- Status should prefer conditions over ad hoc booleans.
- Namespaced references should be explicit when cross namespace use is allowed.
- The DNS contract should be strong enough to support later certificate and proxy work without redesign.

## Exit Criteria

- CRDs install cleanly and reject obviously invalid input.
- Example resources pass schema validation.
- Shared status conventions are documented and used in all four resource types.
- The API shape matches the migration guides closely enough to avoid later schema churn.
