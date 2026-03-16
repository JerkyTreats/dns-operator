# Phase 5: Certificate Delivery Slice

## Goal

Move certificate lifecycle management into a dedicated resource and controller path.

## Scope

- Implement the `Certificate` controller.
- Store issued material in Kubernetes `Secret` resources.
- Model renewal, backoff, and failure states in resource status.
- Integrate DNS challenge behavior with the DNS slice.

## Current Reference Inputs

The current export shows certificate state split across:

- `config.yaml` with provider email, ACME directory URL, and Cloudflare token
- `certificate_domains.json` with one base domain and a SAN list
- shared certificate file paths used by CoreDNS and Caddy

This means the first compatible operator certificate model should preserve the current single certificate plus SAN behavior before attempting more granular certificate ownership.

## Deliverables

- `Certificate` reconciler with issuance and renewal flow.
- Stable secret naming and ownership rules.
- Status fields for issuance, expiry, and last failure.
- Clear dependency contract between `Certificate` and DNS resources.

## Controller Responsibilities

- Watch `Certificate` resources.
- Resolve issuer and challenge credentials through `Secret` references.
- Request and renew certificates.
- Update readiness and expiry conditions.
- Coordinate with DNS when challenge records are required.

## Design Notes

- Keep certificate issuance separate from proxy rollout so each concern is debuggable.
- SAN management should be explicit and reviewable from resource spec and status.
- The controller should avoid implicit domain expansion that users cannot see.
- The first migration target should be one `Certificate` resource for the base domain plus imported SANs.
- Per host certificates can remain a later optimization.

## Exit Criteria

- A `Certificate` can issue successfully and publish a target `Secret`.
- Renewal flow is defined and testable.
- Failure states are visible through conditions and events.
- DNS coordination for challenge records is documented and implemented.
- Import from `certificate_domains.json` preserves the current SAN set.
