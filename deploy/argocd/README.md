# Argo Install Contract

`dns-operator` install artifacts in this repository are intended to be synced by Argo from `deploy/argocd/overlays/<cluster-name>`.

## Sync Order

Recommended sync waves in the infra repository:

1. secret provider applications and materialized `Secret` objects
2. `dns-operator` install artifacts from this repository
3. bootstrap `CertificateBundle` and `TailnetDNSConfig` resources
4. user-facing `PublishedService` and `DNSRecord` resources

## Secret Rules

- Provider credentials must exist before the controller manager starts reconciling dependent resources.
- `CertificateBundle.spec.challenge.cloudflare.apiTokenSecretRef` must point to a `Secret` in the same namespace as the bundle.
- `TailnetDNSConfig.spec.auth.secretRef` must point to a `Secret` in the same namespace as the config object.
- Runtime pods do not consume provider credentials directly.

## Fixed Secret Names

The first cluster overlay assumes stable secret names that are created outside this repository:

- `cloudflare-credentials`
- `tailscale-admin-credentials`

Operator-generated secrets are runtime state and must not be Git-managed:

- `internal-example-test-shared-tls`
- `caddy-runtime-certificates`

## Ownership Boundary

- Argo owns install-time resources, namespace setup, and any intentionally Git-managed custom resources.
- The operator owns rendered runtime `ConfigMap` objects, generated certificate `Secret` objects, and status subresources after install.
