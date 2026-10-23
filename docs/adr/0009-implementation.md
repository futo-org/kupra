# 9. Implementation

- Status: Proposed
- Date: 2026-10-23

## Context

ADRs 1 to 8 decide what kupra does. This ADR records how it's built: language and libraries, how the code is split into components, which Kubernetes conventions kupra follows, and how it's tested, packaged and released.

## Decision

### Language and libraries

kupra is written in Go, on [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime), the library behind Kubebuilder and most Kubernetes operators, Flux and CloudNativePG included. Go is the language of the Kubernetes ecosystem. Most projects kupra works with publish their API types and libraries in Go, and people who contribute to operators usually know it.

kupra's own resources are defined as Go types, and their CRDs and RBAC rules are generated from them with controller-gen, so the types and the schemas can't drift apart.

Other projects' resources are used through the Go types those projects publish. That covers every operator kupra supports or anticipates, and most publish their types as a lightweight module of their own, such as CloudNativePG's `github.com/cloudnative-pg/api`. Some operators kupra might support later aren't written in Go, such as Strimzi, which is written in Java. For those, types are generated from their CRDs, for example with [crd2go](https://github.com/crd2go/crd2go).

Expressions use [cel-go](https://github.com/google/cel-go), together with the CEL libraries Kubernetes adds, so expressions behave exactly as they do in Kubernetes' own validation rules and admission policies. cel-go's type checker does the type checking [ADR 8](0008-templates-and-expressions.md) describes. Readiness is judged with kstatus, the library Flux uses for the same purpose ([ADR 10](0010-lifecycle.md)).

### Components

One repository holds everything, as one Go module plus the dashboard's frontend:

- **Operator:** the controllers for kupra's resources, the rendering engine, and support for each kind of state.
- **Dashboard:** a Go backend that serves a [Svelte](https://svelte.dev) frontend. They're built into one image.
- **CLI:** a kubectl plugin, `kubectl kupra`, distributed through [krew](https://krew.sigs.k8s.io). It's built on the same command-line libraries as kubectl, so flags like `--context` and `--namespace` work the same way. Everyone using the CLI already needs cluster credentials, and usually kubectl, so it meets them where they already work, the way CloudNativePG's `kubectl cnpg` does. The same binary also runs on its own.
- **API types:** one package with kupra's resource types, used by all three. The frontend's TypeScript types are generated from the CRD schemas.

### Kubernetes conventions

kupra applies resources with server-side apply under its own field manager, and uses finalizers for cleanup it has to do itself. Status follows the conditions convention Flux and `kubectl wait` understand: `Ready`, `Reconciling` and `Stalled`, with `observedGeneration`. kupra also records Kubernetes Events for notable changes, and exposes Prometheus metrics.

### Packaging and releases

- **Images:** container images for the operator and dashboard, published to GitHub's container registry for amd64 and arm64.
- **Helm chart:** installs the CRDs, the operator and the optional dashboard, along with the default PreviewPolicy, kupra's ClusterRoles and its admission policies. It's published as an OCI artifact.
- **CLI:** binaries attached to each GitHub release, and published to krew's plugin index.
- **Releases:** made with [release-please](https://github.com/googleapis/release-please), from conventional commits. All components share one version, so there's no compatibility matrix between the operator, dashboard, CLI and chart.

### Tooling and tests

[mise](https://mise.jdx.dev) pins every tool's version (Go, Node, kubectl, kind, Helm, controller-gen and so on) and defines the project's tasks: building, generating CRDs and RBAC, and running each kind of test. CI runs the same tasks as developers do.

There are three layers of tests:

- **Unit tests** cover rendering and state support. Both are pure functions ([ADR 2](0002-state.md)), so this is where most behaviour is tested.
- **Controller tests** run against a real API server, started locally with controller-runtime's envtest, without a cluster.
- **End-to-end tests** run on kind, with the CSI snapshot controller, the CSI hostpath driver and CloudNativePG: the same setup that validated the snapshot design.

### Supported versions

kupra needs Kubernetes 1.36 or newer, for the admission policy that records who created a preview ([ADR 6](0006-access-control.md)), along with the CSI snapshot API and a recent CloudNativePG. Kubernetes 1.34 and 1.35 work too, with the MutatingAdmissionPolicy feature gate enabled.

## Alternatives considered

- **Rust, with kube-rs.** Memory safety and a stronger type system. But kube-rs's runtime has no leader election, and its CEL implementation has neither a type checker nor limits on what an expression may cost to run, which untrusted templates need. It also has no equivalent of envtest, and fewer of the people likely to work on kupra know it.
- **Generating types from CRDs for every project,** with crd2go. One approach for all operators, whatever their language. But nearly every operator kupra needs publishes Go types, and crd2go doesn't yet handle some common schema features, such as int-or-string fields.
- **Handling other projects' resources as unstructured objects.** It works for any project without types, but loses the compiler's checks on exactly the code that's most fiddly.
- **TypeScript.** One language with the frontend, but tooling for Kubernetes operators, and CEL support, are thin.
- **A frontend rendered by the Go backend,** with server-side templates. One language throughout, but forms generated from parameters and live status updates are easier with a frontend framework.
- **A repository per component.** Independent histories, but more overhead keeping types, versions and releases in step.
- **Plain manifests instead of a Helm chart.** Simpler, but most installations, Flux included, consume charts. Rendered manifests can still be attached to releases.
- **A standalone CLI only.** Simpler to install for people without kubectl, which the plugin's binary also covers.

## Consequences

- Go's ecosystem covers what kupra needs: CEL with type checking, envtest and kstatus. If kupra ever renders Helm charts itself, Helm's own Go library is there too.
- kupra's versions of Kubernetes' own libraries have to stay compatible with those of the API modules it imports.
- Supporting a kind of state from an operator that isn't written in Go means generating its types first.
- One version covers every component.
- Clusters older than Kubernetes 1.34 aren't supported.
