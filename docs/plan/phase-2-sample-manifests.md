# Phase 2 Sample Manifests

These examples show the intended resource shape for the first operator API revision.

## Shared Secrets

### Tailscale API key

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tailscale-credentials
  namespace: dns-operator-system
type: Opaque
stringData:
  api-key: tskey-api-example
```

### Cloudflare API token

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: dns-operator-system
type: Opaque
stringData:
  api-token: example-token
```

## `TailscaleDevice`

```yaml
apiVersion: tailscale.jerkytreats.dev/v1alpha1
kind: TailscaleDevice
metadata:
  name: media-server
  namespace: dns-operator-system
spec:
  hostname: media-server
  auth:
    apiKeySecretRef:
      name: tailscale-credentials
      key: api-key
  syncInterval: 5m
  annotations:
    owner: home-lab
    role: media
```

## `DNSRecord` with direct target

```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: grafana
  namespace: dns-operator-system
spec:
  name: grafana
  zone: internal.example.com
  type: A
  ttl: 300
  target:
    value: 100.64.1.20
```

## `DNSRecord` with Tailscale target and proxy intent

```yaml
apiVersion: dns.jerkytreats.dev/v1alpha1
kind: DNSRecord
metadata:
  name: jellyfin
  namespace: dns-operator-system
spec:
  name: jellyfin
  zone: internal.example.com
  type: A
  ttl: 300
  target:
    tailscaleDeviceRef:
      name: media-server
  proxy:
    enabled: true
    targetPort: 8096
    protocol: http
```

## `Certificate`

```yaml
apiVersion: certificate.jerkytreats.dev/v1alpha1
kind: Certificate
metadata:
  name: internal-example-com
  namespace: dns-operator-system
spec:
  domains:
    - internal.example.com
    - '*.internal.example.com'
  issuer:
    provider: letsencrypt-staging
    email: admin@example.com
  challenge:
    type: dns01
    cloudflare:
      apiTokenSecretRef:
        name: cloudflare-credentials
        key: api-token
  secretTemplate:
    name: internal-example-com-tls
  autoRenew: true
  renewBefore: 720h
```

## `ProxyRule`

```yaml
apiVersion: proxy.jerkytreats.dev/v1alpha1
kind: ProxyRule
metadata:
  name: jellyfin
  namespace: dns-operator-system
spec:
  hostname: jellyfin.internal.example.com
  backend:
    address: 100.64.1.20
    port: 8096
    protocol: http
  tls:
    mode: terminate
    certificateRef:
      name: internal-example-com
      namespace: dns-operator-system
  enabled: true
```

## Notes

- These examples assume one operator namespace for the first revision.
- Cross namespace references should remain disabled until there is a clear access model.
- `DNSRecord` and `ProxyRule` can coexist, but DNS should remain the first implementation slice.
