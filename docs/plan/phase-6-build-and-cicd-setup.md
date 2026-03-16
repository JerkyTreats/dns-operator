# Phase 6: Proxy Delivery Slice and Runtime Deployment

## Goal

Manage reverse proxy intent through operator resources and define the runtime deployment contract for CoreDNS and Caddy.

## Scope

- Implement the `ProxyRule` controller.
- Render Caddy configuration into operator owned resources.
- Define how CoreDNS and Caddy consume rendered config.
- Establish deployment manifests for runtime components that the operator feeds.
- Define the Argo install path for the operator manager itself.

## Current Reference Inputs

The current export shows `13` persisted proxy rules in `proxy_rules.json` and a rendered `Caddyfile` with many host blocks using one shared certificate path.

Backends currently include both:

- plain HTTP targets
- HTTPS targets such as `sunshine` that require backend transport TLS handling

## Deliverables

- `ProxyRule` reconciler.
- Caddy config renderer with stable output.
- Deployment, service, and config mount contracts for CoreDNS and Caddy.
- Runtime reload strategy that is safe and observable.
- A documented Git path in this repository that Argo can sync directly.

## Controller Responsibilities

- Watch `ProxyRule` resources.
- Resolve backend targets and certificate linkage.
- Render desired config into `ConfigMap` or `Secret` resources.
- Report whether runtime config is in sync with desired state.

## Design Notes

- Keep runtime processes separate from the operator manager.
- The operator should publish desired config, not embed Caddy or CoreDNS process management.
- Remove local proxy rule file storage from the design.
- The first operator compatible renderer should preserve the current shared certificate pattern for Caddy hosts.
- Migration logic should preserve backend protocol differences from `proxy_rules.json`.
- The install path should be `Kustomize` based and cluster overlay driven.
- Argo should point at this repository for install artifacts, while the infra repository owns the `Application` manifest.

## Exit Criteria

- A `ProxyRule` change updates the expected runtime config artifact.
- Runtime manifests deploy cleanly in a test cluster.
- Caddy reload behavior is defined, measurable, and recoverable.
- No proxy state relies on local files.
- Imported proxy rules produce equivalent host blocks to the current rendered `Caddyfile`.
- A cluster specific overlay can be consumed by Argo without custom build steps.
