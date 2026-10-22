# 9. Implementation

- Status: Proposed
- Date: 2026-10-22

## Context

ADRs 1 to 8 decide what kupra does. This ADR records how it's built: language and libraries, how the code is split into components, which Kubernetes conventions kupra follows, and how it's tested, packaged and released.

## Decision

### Language and libraries

kupra is written in Rust, on [kube-rs](https://kube.rs), a CNCF project that provides a Kubernetes client and a controller runtime. Rust gives memory safety, small static binaries and a type system that catches whole classes of mistakes before they reach a cluster.

kupra's own resources are defined as Rust types, and their CRDs are generated from them, so the types and the schemas can't drift apart. Types for other projects' resources, such as CloudNativePG clusters and VolumeSnapshots, are generated with kopium, from the kube-rs project. kopium reads CRDs from files, so those projects' CRDs are vendored into the repository at pinned versions with [vendir](https://carvel.dev/vendir/).

Expressions are evaluated with [cel-rust](https://github.com/cel-rust/cel-rust). Unlike CEL's Go implementation, it has no static type checker yet. Its maintainers are working towards one, tracked in an [umbrella issue](https://github.com/cel-rust/cel-rust/issues/350), and kupra adopts it when it lands rather than building its own. Until then, kupra checks expressions as [ADR 8](0008-templates-and-expressions.md) describes for that case. cel-rust's function libraries are close to, but not exactly the same as, the extensions Kubernetes adds to CEL, and kupra documents the differences.

### Components

One repository holds everything, as a Cargo workspace plus the dashboard's frontend:

- **Operator:** the controllers for kupra's resources, the rendering engine, and support for each kind of state.
- **Dashboard:** a Rust backend that serves a [Svelte](https://svelte.dev) frontend. They're built into one image.
- **CLI:** a kubectl plugin, `kubectl kupra`, distributed through [krew](https://krew.sigs.k8s.io). Everyone using the CLI already needs cluster credentials, and usually kubectl, so it meets them where they already work, the way CloudNativePG's `kubectl cnpg` does. The same binary also runs on its own.
- **API types:** a shared crate with kupra's resource types, used by all three. The frontend's TypeScript types are generated from the same CRD schemas.

### Kubernetes conventions

kupra applies resources with server-side apply under its own field manager, and uses finalizers for cleanup it has to do itself. Status follows the conditions convention Flux and `kubectl wait` understand: `Ready`, `Reconciling` and `Stalled`, with `observedGeneration`. kupra also records Kubernetes Events for notable changes, and exposes Prometheus metrics.

### Packaging and releases

- **Images:** container images for the operator and dashboard, published to GitHub's container registry for amd64 and arm64.
- **Helm chart:** installs the CRDs, the operator and the optional dashboard, along with the default PreviewPolicy, kupra's ClusterRoles and its admission policy. It's published as an OCI artifact.
- **CLI:** binaries attached to each GitHub release, and published to krew's plugin index.
- **Releases:** made with [release-please](https://github.com/googleapis/release-please), from conventional commits. All components share one version, so there's no compatibility matrix between the operator, dashboard, CLI and chart.

### Tooling and tests

[mise](https://mise.jdx.dev) pins every tool's version (the Rust toolchain, Node, kubectl, kind, Helm, vendir and so on) and defines the project's tasks: building, generating CRDs and third-party types, unit tests and end-to-end tests. CI runs the same tasks as developers do.

Rendering and state support are pure functions ([ADR 2](0002-state.md)), so most behaviour is covered by fast unit tests. End-to-end tests run on kind, with the CSI snapshot controller, the CSI hostpath driver and CloudNativePG: the same setup that validated the snapshot design.

### Supported versions

kupra needs Kubernetes 1.36 or newer, for the admission policy that records who created a preview ([ADR 6](0006-access-control.md)), along with the CSI snapshot API and a recent CloudNativePG.

## Alternatives considered

- **Go with controller-runtime,** the mainstream choice for operators. It has the richer ecosystem: a complete CEL implementation with type checking, test environments that run a real API server without a cluster, and Go client libraries from the operators kupra works with. Rust was chosen for its safety and type system, accepting those gaps.
- **Running CEL's Go implementation for type checking,** for example compiled to WebAssembly. It would provide a complete checker straight away, but adds a second language runtime to the operator. It's a fallback if cel-rust's checker doesn't materialise.
- **A frontend in Rust,** such as Leptos. One language throughout, but a much smaller ecosystem.
- **A standalone CLI only.** Simpler to install for people without kubectl, which the plugin's binary also covers.
- **A repository per component.** Independent histories, but more overhead keeping types, versions and releases in step.
- **Plain manifests instead of a Helm chart.** Simpler, but most installations, Flux included, consume charts. Rendered manifests can still be attached to releases.

## Consequences

- Some things Go operators get for free need work or waiting in Rust: types for third-party resources are generated, and until cel-rust has a type checker, type errors in templates surface on Previews rather than on the PreviewApp.
- End-to-end tests need a kind cluster, rather than a lightweight API server.
- One version covers every component.
- Clusters older than Kubernetes 1.36 aren't supported.
