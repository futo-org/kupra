# 1. kupra: preview environments with first-class state

- Status: Proposed
- Date: 2026-10-15

## Context

Preview environments let people try a change before it ships: a pull request,
a release candidate, the tip of `main`, or a public demo. For anything beyond a
static site, a preview is only useful if it has realistic data. An Immich
preview with an empty library shows almost nothing.

Today, Immich previews run on a hand-built setup. A Flux ResourceSet turns
GitHub pull requests, branches and tags into a CloudNativePG database restored
from a snapshot, a photo library restored from a VolumeSnapshot, and a
HelmRelease. The base snapshots are made by hand from a scratch instance, and
the public demo is reset every few hours by a CronJob running `kubectl`.

It works, but:

- **It's specific to one app.** The state handling is copy-pasted YAML, and
  every new app means building it again.
- **State has no lifecycle.** There's no way to snapshot a preview, reset it,
  start one from another, or refresh the base data except by hand.
- **Only Kubernetes people can use it.** There's nowhere to see which previews
  exist or what state they're in, and no way for anyone else to create one.

Existing preview tooling largely treats previews as stateless deployments, and
leaves the data to seed scripts or to a managed database's branching feature
(see [Alternatives](#alternatives-considered)).

## Decision

We will build **kupra**: a Kubernetes operator, with a web dashboard, that runs
preview environments for any application, with state as a first-class concept.
Previews start from snapshots of realistic data, can be snapshotted
themselves, and are declared as Kubernetes resources, so automation and
humans create them the same way.

### Core model

kupra has three resources:

- **PreviewApp** describes how to run an application: the manifests that make
  up one preview, and which parameters a preview takes.
- **Preview** is one running instance of a PreviewApp, with values for its
  parameters, such as an image tag, and optionally a snapshot to start from.
  Each Preview runs in its own namespace, which kupra creates and removes.
- **PreviewSnapshot** is a captured copy of all of a Preview's state, for
  example its database and volumes taken together. New Previews can start from it.

PreviewApps, their Previews and their PreviewSnapshots live together in a
namespace. That namespace is where people and automation interact with kupra.
The workloads themselves run in the per-preview namespaces.

### Describing an app

A PreviewApp's template is a list of plain Kubernetes manifests: the same
YAML you would write to deploy the app, rendered once per preview. Any field can
be given as an expression instead of a value by adding `Expr` to its name
(`image` becomes `imageExpr`). Expressions are written in
[CEL](https://cel.dev). A few other fields serve the dashboard:

- `parameters` are a preview's inputs, and drive the dashboard's creation form.
- `variables` are values computed once per preview and used in templates.
- `links` are what the dashboard shows for a running preview.

kupra recognises certain kinds of resources as **state**. For each one, it
fills in how the resource bootstraps from a snapshot, and knows how to capture
it into one. Everything else in the template is applied as-is.

A trimmed example:

```yaml
apiVersion: kupra.futo.tech/v1alpha1
kind: PreviewApp
metadata:
  name: immich
spec:
  parameters:
    - name: tag
      title: Image tag
      required: true
  variables:
    - name: host
      expression: preview.name + ".immich.preview.example.com"
  links:
    - name: Web
      urlExpr: '"https://" + variables.host'
  template:
    resources:
      - apiVersion: postgresql.cnpg.io/v1
        kind: Cluster
        metadata:
          name: database
        spec:
          instances: 1
          storage:
            size: 10Gi
      - apiVersion: v1
        kind: PersistentVolumeClaim
        metadata:
          name: library
        spec:
          accessModes: [ReadWriteOnce]
          resources:
            requests:
              storage: 100Gi
      - apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: immich-server
        spec:
          template:
            spec:
              containers:
                - name: server
                  imageExpr: '"ghcr.io/immich-app/immich-server:" + parameters.tag'
      # ...Services, an HTTPRoute, and so on
---
apiVersion: kupra.futo.tech/v1alpha1
kind: Preview
metadata:
  name: pr-1234
spec:
  appRef:
    name: immich
  parameters:
    tag: pr-1234
  restoreFrom:
    snapshotSelector:
      matchLabels:
        kupra.futo.tech/channel: base
---
apiVersion: kupra.futo.tech/v1alpha1
kind: PreviewSnapshot
metadata:
  name: immich-base-2026-10-15
  labels:
    kupra.futo.tech/channel: base
spec:
  source:
    previewRef:
      name: main
```

Complete examples are in [`docs/examples`](../examples).

### State and snapshots

A Preview starts empty unless it names a snapshot to start from, either
directly or by label. In the example, the Preview asks for the newest ready
snapshot labelled `channel: base`, so publishing new base data only takes
creating a new snapshot. Choosing a snapshot is up to whatever creates the
Preview: a ResourceSet template, a person in the dashboard, a script.

Before a preview's workloads start, kupra restores each piece of state from
the snapshot. A preview can also be reset to where it started, which is how
the public demo stays clean.

Support for state is designed to be extended. Each kind of state is a
self-contained piece of kupra that knows three things: how to recognise the
resource, how to make it start from a snapshot, and how to capture it. kupra
works through each tool's own interface wherever there is one. CloudNativePG
clusters, for example, are captured and restored through CloudNativePG's
backup and recovery resources, not by snapshotting their volumes behind its
back. The first version supports CloudNativePG clusters and plain
PersistentVolumeClaims. Object storage buckets, generated secrets that must
travel with the data, and other databases are expected to follow.

### Principles

- **No app-specific code.** Everything an app needs is in its PreviewApp. If
  supporting an app requires changing kupra, the missing piece should be a
  general one.
- **Integrate, don't reimplement.** kupra provides hooks and leaves adjacent
  features to existing tools:

  | Concern | Handled by |
  | --- | --- |
  | Creating previews from git (PRs, branches, tags) | anything that can create a Preview, e.g. a Flux ResourceSet |
  | Routing, TLS, auth in front of previews | the template, e.g. Gateway API HTTPRoutes |
  | Secrets from external stores | the template, e.g. External Secrets |
  | Scale-to-zero | a `/scale` subresource on Preview, driven by e.g. KEDA |
  | Waiting on previews | standard status conditions |
  | Access control | Kubernetes RBAC, with OIDC for the dashboard |

- **Isolation.** Each preview runs in its own namespace, and kupra treats both
  sides of it as untrusted. A PreviewApp's template can describe arbitrary
  resources, and a preview of a pull request runs code nobody has reviewed yet.
  So kupra confines what a template produces to the preview's namespace, and
  manages network policies and quotas around it. The namespace also gives clean
  teardown and a natural boundary for access.
- **Kubernetes-native.** All state lives in Kubernetes resources. The dashboard
  is a client of the API like any other, so anything it can do, GitOps can do.

### Scope

**Minimum viable product:** parity with the current Immich setup, plus a
dashboard.

- PreviewApp, Preview and PreviewSnapshot.
- CloudNativePG clusters and PVCs as state.
- Previews started empty or from a snapshot, chosen by name or label;
  snapshots taken from a running preview; reset to where a preview started.
- Namespace isolation, including network policies.
- A dashboard to browse previews and their links, create one from a form,
  delete one, and take a snapshot, behind an OIDC sign-in.

**Designed for, but later:**

- cloning a running preview
- portable snapshots that can be exported out of the cluster, for example
  into a local development environment, and imported back
- more kinds of state (buckets, generated secrets)
- sleep and scale-to-zero
- expiry
- status reporting back to GitHub
- limits on concurrent previews
- scheduled refreshes of base snapshots
- access to running previews (logs, exec, and a database browser such as
  CloudBeaver, already connected to the preview's databases)

**Non-goals:**

- building images or running CI
- deploying production
- provisioning clusters or virtual clusters
- sanitising data: previews are only as safe as the snapshots you give them

## Alternatives considered

**Keep extending Flux ResourceSets.** ResourceSets template resources from
inputs well, and kupra is designed to sit behind one. But they're stateless
by design: when an input disappears its resources are pruned, with no hook to
snapshot first, no ordering, and no per-instance status. A Flux
ResourceSetInputProvider of type `ExternalService` only adds another source of
inputs, which leaves the same gaps.

**GitOps generators** such as Argo CD's ApplicationSet pull request generator
are in the same position as ResourceSets. Writeups using them leave the
database to the reader: an empty one per preview, a schema per preview in a
shared database, or an external branching service like
[Neon](https://neon.com/blog/preview-environments-neon-kubernetes-argo-cd).

**Preview environment platforms** (Okteto, Bunnyshell, Release, Qovery,
Northflank, Signadot) are mostly commercial, and centred on git hosting
integration. Their data support varies:

- [Okteto](https://www.okteto.com/docs/core/use-volume-snapshots/) restores
  volumes from snapshots, which is close to our approach.
- Release builds database pools on cloud-managed databases.
- Qovery clones an environment's configuration but not its data.
- Signadot avoids copying data altogether by routing requests to changed
  services.

None is a self-hosted, open, Kubernetes-native tool that treats a whole
preview's state as one unit.

**Self-hosted PaaS with review apps** ([Kubero](https://github.com/kubero-dev/kubero),
and Uffizzi, which has been inactive since 2024) give previews fresh add-on
databases rather than realistic data.

**Data management tools** ([Velero](https://velero.io),
[Kanister](https://kanister.io)) can back up a namespace and restore it
somewhere else, or run application-aware capture and restore steps. They're
building blocks rather than preview systems: they know nothing about apps,
parameters or lifecycle.

**Composition tools** ([kro](https://kro.run), Crossplane) render groups of
resources from a template, much like a PreviewApp, but have no notion of state.

The pieces all exist. What's missing is a tool that combines them around state.

## Consequences

- Each app needs one PreviewApp, rather than its own bespoke setup.
- Snapshots, restores and resets become declarative and visible, instead of
  scripts and manual steps.
- We take on an operator and a dashboard to maintain.
- Each kind of state needs specific support. Apps whose state kupra doesn't
  recognise still run, but their data isn't captured or restored.
- kupra depends on CSI volume snapshots, and on the backup and recovery APIs
  of the operators it supports, starting with CloudNativePG.
- App definitions are plain manifests. Apps packaged as Helm charts can
  include a Flux HelmRelease in their template, but for now kupra treats it as
  an opaque resource and can't manage state created inside the chart.

## Later decisions

Most of these have already been explored, and are left out here to keep this
document focused. Each will get its own ADR:

- The expression language and template rendering in detail.
- The interface for supporting a kind of state, and whether it can be extended
  out of tree.
- How snapshots are taken consistently and made available to preview
  namespaces.
- How preview namespaces are named.
- How templates are confined: the permissions they're applied with, default
  network policies, quotas.
- Sleep and scale-to-zero.
- Access control, for the dashboard and for running previews.
- The GitHub integration.
- Implementation choices: language, tooling, and how the dashboard is built.

## Questions for reviewers

- Does the three-resource model fit how you'd want to run previews?
- Is plain manifests as the app definition a problem for you, compared to Helm
  charts?
- After Postgres and volumes, which kinds of state matter most to you?
- What would you need from the dashboard?
- Is there anything here that rules kupra out for your setup, such as
  storage without snapshot support?
