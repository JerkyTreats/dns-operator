# Reference Import Tool

## Goal

Create a rerunnable import path from the current exported runtime data into the operator-managed resource set.

## Supported Inputs

The importer currently accepts:

- `config.yaml`
- zone file
- `proxy_rules.json`
- `certificate_domains.json`
- optional rendered `Caddyfile`
- current Tailscale restricted nameserver address via a CLI flag

## Output

The tool emits deterministic Kubernetes YAML for:

- `Secret`
- `DNSRecord`
- `PublishedService`
- `CertificateBundle`
- `TailnetDNSConfig`

It can also emit a JSON report with:

- imported object counts
- skipped disabled proxy rules
- case-collision notes
- migration warnings

## Usage

```sh
go run ./cmd/import-reference \
  --namespace dns-operator-system \
  --config /path/to/export/config.yaml \
  --zone-file /path/to/export/internal.example.test.zone \
  --proxy-rules /path/to/export/proxy_rules.json \
  --certificate-domains /path/to/export/certificate_domains.json \
  --caddyfile /path/to/export/Caddyfile \
  --nameserver-address 192.0.2.53 \
  --output dist/imported-resources.yaml \
  --report dist/import-report.json
```

## Import Behavior

- Hostnames are normalized to lower case.
- Case-only collisions are preserved in the report instead of silently ignored.
- Disabled proxy rules are reported and omitted from desired published state.
- `tls_insecure_skip_verify` is inferred from the rendered `Caddyfile` when present.
- The shared certificate bundle defaults to `internal-shared`.
- The importer only emits `TailnetDNSConfig` when both the tailnet and current nameserver address are known.

## Safety Notes

- The importer is idempotent as long as the same source inputs are supplied.
- Provider credentials are moved into Kubernetes `Secret` objects and never emitted into CR specs.
- The current Tailscale DNS admin state is still an external migration input, so the nameserver address must be captured explicitly during cutover planning.
