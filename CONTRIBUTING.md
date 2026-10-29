# Contributing

kupra's design is described in its [ADRs](docs/adr), starting with [ADR 1](docs/adr/0001-kupra.md).

## Development

Tools are pinned with [mise](https://mise.jdx.dev). After `mise install`:

- `mise run check` lints and runs the unit tests.
- `mise run generate` generates deepcopy code, CRDs and RBAC from the API types and `+kubebuilder` markers. CI fails if generated files are out of date. The operator's RBAC goes to `charts/kupra/files/role.yaml`, and the chart puts its Role in the release's namespace, so `namespace=kupra-system` in markers means the namespace kupra is installed in.
- `mise run test:integration` runs the controller tests against a local API server, with envtest.
- `mise run e2e:up` creates a kind cluster with the CSI snapshot controller, the CSI hostpath driver and CloudNativePG. It also installs the chart, with an operator image built by [ko](https://ko.build) from the working tree. `mise run test:smoke` checks that the installed operator reconciles a Preview. `mise run test:e2e` scales it down and runs the end-to-end tests on the cluster, and `mise run e2e:down` deletes it.

There are three layers of tests:

- **Unit tests** cover rendering and anything else written as pure functions, which is where most behaviour should be tested.
- **Integration tests** run against envtest, which is a real API server without Kubernetes' controllers. Namespaces never finish deleting there, and ClusterRoles aren't aggregated.
- **End-to-end tests** run the operator in the test process, but with its own service account's token, so its RBAC is tested too.

## Conventions

- Where Kubernetes has a standard shape for something, the API uses it, such as discriminated unions, `corev1` reference types and `rbacv1.Subject`.
- Before writing a type or helper, check whether Kubernetes, controller-runtime, apiserver or Flux's libraries already have one. If none fits, say why in a comment.

## Prose in code

kupra's prose reads like Kubernetes' own. When in doubt, find how Kubernetes, controller-runtime or Flux words the same thing, and copy it.

- **Errors and messages,** such as field errors, condition messages and events, are terse: lowercase, no trailing period, one clause. They say what's wrong, not why or how to fix it, in the wording of apimachinery's field errors: `is required`, `must be ...`, `may not be set`, `not found`. For example: `spec.dataSourceRef: Forbidden: may not be set on a claim that's state`.
- **Doc comments** start with the name and say what it is or does, in one sentence. A second sentence is only for what a caller can't tell from the signature.
- **Comments** say why, where the code can't. They don't narrate what the code does, compare it with earlier versions, or argue for the design: that belongs in the ADRs.
- **CRD descriptions and flag help** are one short sentence, like those of Kubernetes' own types.
- No filler, such as "note that", "simply" or "just", and no hedging.

## Commits and pull requests

- Commit messages are [conventional commits](https://www.conventionalcommits.org), one line, with no body.
- PRs are squash-merged under their title, which is a conventional commit too.
- Larger work is a stack of PRs, managed with `gh stack`. Each commit in a PR builds and passes the tests on its own.
- Kritika, the org's review bot, reviews every PR. Once it's happy, a maintainer reviews. From then on, fixes go in as new commits instead of rewriting the PR's history.

## ADRs

An ADR stays Proposed until it's implemented. The PR that finishes the last part of an ADR marks it Accepted, and updates it if the implementation turned out differently.

## Releases

release-please opens a release PR from the conventional commits on `main`. Merging it tags the release, and publishes the operator's image to `ghcr.io/futo-org/kupra-operator` and the chart to `oci://ghcr.io/futo-org/charts/kupra`, all with the same version.
