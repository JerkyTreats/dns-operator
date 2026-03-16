# Phase 4: Tailscale Delivery Slice

## Goal

Move device discovery and IP resolution into Kubernetes resources and controller status.

## Scope

- Implement the `TailscaleDevice` controller.
- Replace file backed device state with resource status.
- Resolve `DNSRecord` targets through `TailscaleDevice` references.
- Define polling, backoff, and rate limit behavior for the Tailscale API.

## Current Reference Inputs

The current export stores Tailscale device state in `devices.json` as a simple list of name, Tailscale IP, and description values. It does not include online state in the persisted data.

The current config also enables polling with an interval of `1h`.

## Deliverables

- `TailscaleDevice` reconciler with periodic requeue behavior.
- `Secret` based credential handling for Tailscale API access.
- Status fields for current device id, current IP, sync time, and failures.
- `DNSRecord` integration that can resolve device based targets.

## Controller Responsibilities

- Watch `TailscaleDevice` resources.
- Poll the Tailscale API on a controlled schedule.
- Update status without writing separate persistence files.
- Surface offline, missing, and rate limited states clearly.
- Trigger `DNSRecord` reconciliation when device IP changes affect records.

## Design Notes

- A `TailscaleDevice` should not create `DNSRecord` resources automatically.
- Device polling policy belongs in the resource spec or shared operator config.
- Reference identity should be stable enough to survive IP changes.
- Import logic should not assume source data contains online status.
- The first migration pass should preserve imported descriptions and names even when they need hostname normalization later.

## Exit Criteria

- `TailscaleDevice` status reflects current API state.
- `DNSRecord` resources can resolve a target from device status.
- No JSON device store remains in the operator path.
- Error handling covers missing credentials, missing devices, and API throttling.
- Import from the current `devices.json` format works without lossy field drops.
