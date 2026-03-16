# Current Reference Migration Matrix

## Purpose

This document maps the exported reference runtime data into the target operator resources.

It exists to answer one question clearly:

What should be imported from each current file, into which Kubernetes resource, with which field mapping and migration rules.

## Input Files

- `config.yaml`
- `devices.json`
- `proxy_rules.json`
- `certificate_domains.json`
- zone file
- `Corefile`
- `Caddyfile`

## Import Order

The import path should run in this order:

1. shared secrets and operator config
2. `TailscaleDevice` resources
3. `DNSRecord` resources for direct records
4. `DNSRecord` resources for proxy fronted service names
5. one shared `Certificate` resource for the current base domain and SAN set
6. `ProxyRule` resources
7. rendered runtime verification against CoreDNS and Caddy output

## Mapping Rules

### `config.yaml`

**Target resources**

- operator `ConfigMap`
- operator `Secret`
- shared `Certificate`
- default runtime deployment values

**Field mapping**

- `dns.domain` to operator default managed zone
- `dns.internal.origin` to operator default zone origin
- `dns.internal.polling.interval` to default `TailscaleDevice` sync interval when a resource does not override it
- `tailscale.api_key` to `Secret` named `tailscale-credentials`, key `api-key`
- `tailscale.tailnet` to operator config default for Tailscale resources
- `tailscale.device_name` to migration metadata only, not to a long term shared controller setting
- `certificate.email` to `Certificate.spec.issuer.email`
- `certificate.cloudflare_api_token` to `Secret` named `cloudflare-credentials`, key `api-token`
- `certificate.use_production_certs` to `Certificate.spec.issuer.provider`
- `certificate.renewal.renew_before` to `Certificate.spec.renewBefore`

**Migration notes**

- Secrets must never stay in plain text config after import.
- `ca_dir_url` can remain an implementation detail if provider selection already captures production versus staging.
- Current file paths for certs and runtime config should become deployment wiring, not user managed spec fields.

### `devices.json`

**Target resources**

- one `TailscaleDevice` per source device

**Field mapping**

- source `name` to `spec.hostname`
- normalized source `name` to `metadata.name`
- source `description` to a migration annotation such as `migration.example.io/source-description`
- operator default tailnet from imported config to `spec.tailnet` when needed
- imported Tailscale credential secret ref to `spec.auth.apiKeySecretRef`

**Migration notes**

- Source data does not contain online state, so import should not attempt to write `status.online`.
- Source `tailscale_ip` should not be written to status during import. The controller should populate status after first sync.
- The importer should record the original source name in annotations when normalization changes it.
- Names with punctuation or spaces need normalized Kubernetes names while preserving the original lookup value in spec.

### zone file

**Target resources**

- one `DNSRecord` per desired hostname after normalization and dedupe

**Field mapping**

- zone label to `DNSRecord.spec.name`
- zone origin to `DNSRecord.spec.zone`
- record type to `DNSRecord.spec.type`
- record value to either `DNSRecord.spec.target.value` or `DNSRecord.spec.target.tailscaleDeviceRef`

**Classification rules**

- If a hostname maps to a source device IP and has no proxy rule, import it as a direct device backed record when there is a clear device match.
- If a hostname has a proxy rule and the zone points at the current ingress host IP, import the `DNSRecord` as the public DNS name for the proxied service, preserving current ingress behavior.
- If a hostname has no device match and no proxy rule, import it as a literal target record and flag it for review.

**Migration notes**

- Nested labels must remain valid import targets.
- DNS is case insensitive, so labels that differ only by case represent one desired hostname and require dedupe.
- The importer should default to lower case host labels and emit a collision report when source labels differ only by case.

### `proxy_rules.json`

**Target resources**

- one `ProxyRule` per stored hostname

**Field mapping**

- source key or `hostname` to `ProxyRule.spec.hostname`
- `target_ip` to `ProxyRule.spec.backend.address`
- `target_port` to `ProxyRule.spec.backend.port`
- `protocol` to `ProxyRule.spec.backend.protocol`
- `enabled` to `ProxyRule.spec.enabled`

**Derived fields**

- shared certificate linkage to `ProxyRule.spec.tls.certificateRef`
- TLS mode should default to the shared termination model used today

**Migration notes**

- Backend protocol differences must be preserved.
- HTTPS backends such as `sunshine` require explicit transport handling in the future renderer.
- `created_at` is useful migration metadata and should move to an annotation instead of spec.

### `certificate_domains.json`

**Target resources**

- one initial shared `Certificate`

**Field mapping**

- `base_domain` to the first entry in `Certificate.spec.domains`
- each `san_domains` entry to additional `Certificate.spec.domains` values
- imported email and provider config from `config.yaml` to `Certificate.spec.issuer`
- imported Cloudflare token secret ref to `Certificate.spec.challenge.cloudflare.apiTokenSecretRef`

**Migration notes**

- The first compatible operator state should preserve the current single certificate plus SAN model.
- Per host certificate splitting can happen later after functional parity is proven.
- Domain import should be deduped case insensitively.

### `Corefile`

**Target resources**

- runtime deployment defaults
- verification data for generated CoreDNS config

**Migration notes**

- Treat `Corefile` as a rendered artifact to compare against, not as the main source of desired state.
- Current health and TLS listener behavior should be preserved in the runtime deployment plan.

### `Caddyfile`

**Target resources**

- runtime deployment defaults
- verification data for generated Caddy config

**Migration notes**

- Treat `Caddyfile` as a rendered artifact to compare against, not as the main source of desired state.
- The current shared certificate path pattern should remain the first compatible render target.

## Resource Count Expectations

Based on the exported snapshot, the first import should expect roughly:

- multiple `TailscaleDevice` resources
- multiple `DNSRecord` resources before dedupe and classification
- multiple `ProxyRule` resources
- one shared `Certificate` resource with the imported domain set

## Known Edge Cases

### Case collisions

- mixed case and lower case variants of the same hostname

The importer should produce one lower case canonical DNS name and keep source variants in annotations or a migration report.

### Name normalization

Examples from the export that need careful handling:

- device source names with spaces
- device source names with punctuation
- nested DNS labels within the managed zone

### Shared ingress behavior

Many service DNS names currently point to the DNS manager host IP and rely on Caddy for the backend hop. The first operator migration should preserve that traffic pattern.

## Validation Checks

After import, validate:

- all imported hostnames resolve to the expected rendered DNS targets
- all imported proxy hosts render equivalent backend mappings
- all SAN domains appear on the imported shared certificate resource
- no source credentials remain in plain text config used by the new operator runtime
- case collisions and normalization changes are captured in a migration report

## Decision Summary

- `config.yaml` seeds operator config and secrets
- `devices.json` seeds `TailscaleDevice`
- zone file seeds `DNSRecord`
- `proxy_rules.json` seeds `ProxyRule`
- `certificate_domains.json` seeds one shared `Certificate`
- `Corefile` and `Caddyfile` are verification artifacts, not primary desired state inputs
