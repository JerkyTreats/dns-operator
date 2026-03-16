# Phase 1: Foundation and Control Loop

## Goal

Create the operator foundation with a working manager process, code generation flow, and local cluster loop.

## Scope

- Initialize the Kubebuilder project.
- Set the repository domain, image naming, and manager defaults.
- Add health, readiness, and metrics endpoints.
- Establish local cluster workflow with `kind` or another disposable cluster.
- Verify generate, manifests, test, and run commands from the start.
- Create the deploy artifact layout that Argo will consume.

## Deliverables

- Root operator scaffold with `main.go`, `PROJECT`, `Makefile`, `go.mod`, and generated config.
- Manager process that starts cleanly with no domain logic yet.
- Local development instructions for install, run, and cleanup.
- Repository conventions for namespaces, labels, and controller package layout.
- `deploy/argocd/base` and `deploy/argocd/overlays/cluster-name` as the first runtime install surface.

## Key Decisions

- Use the controller runtime manager as the only long lived control loop.
- Do not import reference packages directly into the operator manager path.
- Keep bootstrap minimal so the first real domain slice can land quickly.
- Treat deploy manifests as a stable interface for Argo from the start.
- Prefer `Kustomize` over `Helm` for the first install path.

## Exit Criteria

- `make generate` and `make manifests` run successfully.
- `make test` runs with the empty scaffold.
- `make run` starts the manager against a local cluster.
- Generated manifests install into the cluster without manual edits.
- The Argo consumable deploy path exists and can be targeted by a Git path application.

## Deferred Work

- Domain specific controllers
- Migration tooling
- Production hardening beyond basic manager defaults
