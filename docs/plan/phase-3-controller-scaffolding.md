# Phase 3: DNS Delivery Slice

## Goal

Land the first end to end controller path with `DNSRecord` as the primary user facing resource.

## Scope

- Implement the `DNSRecord` reconciler.
- Aggregate records by zone and render zone data into `ConfigMap` resources.
- Reuse proven validation and rendering behavior from the reference implementation where it fits the controller model.
- Publish clear status, events, and error handling for record lifecycle.

## Current Reference Inputs

The current export shows one live zone with `28` A records. Migration logic for this phase should consume:

- `config.yaml` for the managed domain
- the current zone file for current record data
- `proxy_rules.json` to distinguish proxy fronted services from direct records

This matters because many service names currently resolve to the DNS manager host IP first, then rely on Caddy for the real backend hop.

## Deliverables

- `DNSRecord` controller with create, update, and delete reconciliation.
- Zone rendering package that produces stable output for a zone from the current set of records.
- `ConfigMap` contract for CoreDNS consumption.
- Sample manifests that prove multi record aggregation into one zone artifact.

## Controller Responsibilities

- Watch `DNSRecord` resources.
- Resolve target inputs that do not depend on external device lookup yet.
- Group records by zone.
- Write rendered zone data only when effective output changes.
- Update status with resolved record state and render results.

## Design Notes

- Treat the rendered zone as a shared artifact owned by the operator.
- Avoid direct runtime reload logic in the first DNS slice.
- Do not block the DNS slice on certificate or proxy support.
- Keep finalizer behavior simple and tied to zone cleanup only when needed.
- Preserve current behavior for direct device records and proxy fronted service records during migration.
- Accept nested record labels.
- Normalize or explicitly reject case only collisions during import.

## Exit Criteria

- Creating a `DNSRecord` updates the expected zone artifact.
- Updating a record changes only the affected zone output.
- Deleting a record removes it from the zone output.
- Status and events make failures diagnosable without reading controller code.
- Imported records from the current zone file round trip without unexpected hostname drift.
