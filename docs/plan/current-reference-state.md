# Current Reference State

## Purpose

This document records the observed state of the current reference DNS manager from the exported runtime data. The roadmap should stay aligned with this inventory so migration work targets the real system, not an abstract design.

## Source Snapshot

Observed from a local runtime export stored outside the repository.

## Runtime Layout

- Main configuration file at `configs/config.yaml`
- CoreDNS config at `coredns/Corefile`
- CoreDNS zone data at `coredns/zones/<zone>.zone`
- Device inventory at `data/devices.json`
- Certificate domain state at `data/certificate_domains.json`
- Proxy rule state at `data/proxy_rules.json`
- Rendered Caddy config at `configs/Caddyfile`

## Observed Inventory

- One managed zone
- multiple DNS A records in the zone file
- multiple stored Tailscale devices
- multiple stored proxy rules
- multiple certificate SAN domains

## DNS Reality

- The current system serves one zone from a single zone file.
- Some names map directly to device IPs.
- Many service names map to the DNS manager host IP and rely on Caddy for the final backend hop.
- The zone includes mixed case names and lower case equivalents.
- The zone also includes nested labels.

## Tailscale Reality

- Device state is persisted as a simple list with name, Tailscale IP, and description.
- The exported device file does not carry online state.
- Polling is enabled in config with an interval of `1h`.
- The current runtime also stores raw Tailscale credentials in `config.yaml`.

## Certificate Reality

- The current runtime appears to use one base domain certificate plus a SAN list.
- Caddy is configured to use the same existing certificate path for many hosts.
- Certificate domain membership is tracked in `certificate_domains.json`.
- The current runtime also stores the Cloudflare token in `config.yaml`.

## Proxy Reality

- Proxy rules are persisted as hostname to backend mappings with target IP, target port, protocol, enabled flag, and created time.
- Rendered Caddy config is file based.
- Proxy backends include both plain HTTP and HTTPS.
- At least one backend uses HTTPS with insecure verification disabled at the backend hop.

## Migration Implications

- Migration tooling must read appdata style file layouts, not just source repo structures.
- DNS migration must preserve the distinction between direct device records and proxy fronted service records.
- Certificate migration should preserve the current single certificate plus SAN model as the first compatible operator slice.
- Tailscale migration should accept that source data may not include online state.
- Import logic must normalize or flag case collisions in host labels.
- Secret migration is mandatory because current config stores provider credentials in plain text.

## Planning Constraints

- The roadmap should treat `config.yaml`, `devices.json`, `proxy_rules.json`, `certificate_domains.json`, and the zone file as first class migration inputs.
- The first operator milestone should preserve current behavior before introducing more granular certificate or proxy models.
- Validation and migration steps must account for nested host labels, case variants, and shared certificate usage.
