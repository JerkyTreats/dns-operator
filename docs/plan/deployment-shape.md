# Deployment Shape

## Goal

Define how `dns-operator` is delivered into the cluster through Argo while keeping Argo focused on install time artifacts and the operator focused on runtime managed state.

## Recommended Model

- Argo applications remain defined in the infra repository.
- This repository contains the runtime install artifacts that Argo applies.
- The cluster runs a published container image for `dns-operator`.
- Argo reads deploy manifests from this repository, but does not build the image and does not execute source code.
- The operator owns generated runtime resources after installation.

## Repository Split

### Infra repository

The infra repository should own:

- the Argo `Application` manifest for `dns-operator`
- cluster specific ordering through sync waves
- shared secret provider applications
- optional Git managed custom resources such as `DNSRecord` and `Certificate` later on

### `dns-operator` repository

This repository should own:

- CRDs
- RBAC
- `ServiceAccount`
- manager `Deployment`
- service and monitoring manifests
- static operator config defaults
- cluster overlays for image tags and deployment settings

## Directory Shape

The first clean install surface should look like this:

```text
deploy/
  argocd/
    base/
      namespace.yaml
      crds/
      rbac/
      manager-deployment.yaml
      service.yaml
      kustomization.yaml
    overlays/
      cluster-name/
        kustomization.yaml
        image-tag-patch.yaml
        config-patch.yaml
        secret-ref-patch.yaml
```

## Why `Kustomize` First

- It matches Argo Git path deployment cleanly.
- It keeps the install surface transparent while the operator design is still changing.
- It avoids a second packaging layer before the runtime contracts settle.
- It handles base and cluster overlay composition cleanly.

`Helm` can be added later if the operator needs a broader install interface across many clusters.

## Argo Ownership Boundary

Argo should own:

- namespace creation
- CRD installation
- RBAC and service accounts
- operator deployment
- static config and secret references
- optional bootstrap custom resources that are intentionally Git managed

Argo should not own:

- rendered zone `ConfigMap` output
- rendered Caddy config output
- issued certificate `Secret` output
- status subresources
- any generated child resources that the operator reconciles and updates continuously

## Secret Flow

- Secret provider applications should sync before `dns-operator`.
- `dns-operator` should reference Kubernetes `Secret` objects, not raw values.
- The current plain text Tailscale and Cloudflare credentials must migrate into `Secret` objects before cutover.

## Image Flow

- CI builds and publishes the `dns-operator` image.
- CI updates the image tag in the cluster overlay in this repository.
- Argo detects the Git change and syncs the updated deployment.

This keeps deployment intent in Git while keeping image build concerns out of Argo.

## Sync Wave Guidance

Recommended ordering:

1. secret provider and secret material
2. `dns-operator` install artifacts
3. optional Git managed `DNSRecord`, `TailscaleDevice`, `Certificate`, and `ProxyRule` resources

## Custom Resource Ownership

Two modes can coexist over time:

- migration mode, where import jobs create resources from the current reference export
- GitOps mode, where selected custom resources are committed in the infra repository and applied by Argo

The key boundary is that Argo may create desired resources, but the operator owns their status and any generated runtime artifacts.

## First Cluster Target

The first target should be a single cluster overlay with:

- fixed namespace choices
- fixed secret names
- fixed image tag pinning
- no `Helm` packaging layer

This keeps the first deployment path simple while the operator reaches parity with the current reference runtime.
