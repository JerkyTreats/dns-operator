# Operator Delivery Roadmap

## Goal

Build a Kubernetes operator that replaces the reference DNS manager with resource driven reconciliation, status driven feedback, and Kubernetes native runtime deployment.

## Planning Principles

- Deliver one vertical slice at a time, starting with DNS.
- Make Kubernetes resources the source of truth from the first controller onward.
- Keep the reference app as a behavior guide, not as a runtime dependency.
- Treat migration, rollback, and observability as core work, not cleanup work.
- Avoid scaffolding every domain before the first real slice proves out.

## Phase Order

- [Phase 1: Foundation and control loop](phase-1-project-initialization.md)
- [Phase 2: Resource contracts and status model](phase-2-core-crd-definitions.md)
- [Phase 3: DNS delivery slice](phase-3-controller-scaffolding.md)
- [Phase 4: Tailscale delivery slice](phase-4-development-environment-setup.md)
- [Phase 5: Certificate delivery slice](phase-5-testing-infrastructure.md)
- [Phase 6: Proxy delivery slice and runtime deployment](phase-6-build-and-cicd-setup.md)
- [Phase 7: Security, secrets, and RBAC](phase-7-rbac-and-security.md)
- [Phase 8: Observability and operational readiness](phase-8-observability.md)
- [Phase 9: Testing, migration tooling, and release flow](phase-9-documentation.md)
- [Phase 10: Cutover and production validation](phase-10-validation.md)

## Delivery Shape

The roadmap is intentionally front loaded toward DNS because DNS is the narrowest path from the reference implementation to a useful operator outcome. Once that slice is stable, Tailscale, certificate, and proxy work can layer on top without reworking the core contract.

Each phase should produce four things:

- clear API shape
- a runnable reconciliation path
- status and event feedback
- an upgrade and rollback story

## Cross Phase Decisions

- Shared condition names should be defined once and reused across all resources.
- Secret material must enter the system through `Secret` references, never raw spec fields.
- File persistence from the reference app should be treated as migration input only.
- Runtime config should be rendered into `ConfigMap` or `Secret` resources that CoreDNS and Caddy consume.
- Any optional HTTP compatibility layer should remain outside the core controller design.
- Argo should install `dns-operator` from deploy artifacts in this repository, while the operator owns generated runtime state.
- The initial install surface should use `Kustomize` overlays instead of a `Helm` chart.

## Observed Reference Reality

The current reference system is not just source code in `reference/`. It also has a live runtime data shape captured in a local export that is intentionally kept outside the repository.

- One active managed zone
- persisted Tailscale device state
- persisted proxy rule state
- persisted certificate SAN state
- one shared certificate pattern used across many Caddy hosts
- plain text provider credentials in the legacy runtime config

The detailed source of truth for planning is in [Current reference state](current-reference-state.md).

The concrete source to target import mapping is in [Current reference migration matrix](current-reference-migration-matrix.md).

The target Argo deployment model is in [Deployment shape](deployment-shape.md).

## Success Criteria

The operator plan is complete when:

- `DNSRecord`, `TailscaleDevice`, `Certificate`, and `ProxyRule` have stable schemas and status contracts.
- DNS, certificate, proxy, and Tailscale state are managed through Kubernetes resources.
- Core runtime artifacts are rendered from operator owned resources.
- Migration tooling exists for reference data and is safe to rerun.
- Validation, tests, metrics, and rollback guidance exist for production cutover.

## Reference Inputs

- Research summaries in `docs/research/`
- Domain migration guides in `docs/migration/`
- Runtime behavior in `reference/`
- Exported runtime state in `docs/plan/current-reference-state.md`
- Source to target migration mapping in `docs/plan/current-reference-migration-matrix.md`
- Argo install model in `docs/plan/deployment-shape.md`
