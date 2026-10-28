# 8. Templates and expressions

- Status: Proposed
- Date: 2026-10-22

## Context

[ADR 1](0001-kupra.md) describes an app as a template of plain Kubernetes manifests, rendered once per preview, with `parameters`, `variables` and `links` alongside it, and expressions in fields whose name ends in `Expr`. This ADR fixes the rules: what parameters look like, how expressions and variables work, what they can refer to, and how a rendered template is applied and kept up to date.

## Decision

### Parameters

A PreviewApp's `parameters` are an ordered list. Each has a `name`, a `title` and optionally a `description`. Its `type` is `string` (the default), `integer` or `boolean`, optionally limited by an `enum`, with an optional `default`, and it can be `required`. Values are given as strings, like environment variables, and parsed by type. A `uiHint` asks the dashboard for a special picker, such as an image's tags. The order is the order of the dashboard's form.

```yaml
parameters:
  - name: tag
    title: Image tag
    required: true
    uiHint:
      type: imageTag
      imageTag:
        repository: ghcr.io/immich-app/immich-server
  - name: logLevel
    title: Log level
    enum: [verbose, debug, log]
    default: log
  - name: machineLearning
    title: Machine learning
    type: boolean
    default: "true"
```

This is deliberately much smaller than a full JSON schema. It's shaped by what a creation form needs, much like GitHub's issue forms. kupra checks a Preview's parameters against them and reports problems on the Preview's status. The dashboard checks them before submitting.

### Expressions

Expressions are written in [CEL](https://cel.dev), the language Kubernetes itself uses for validation rules and admission policies. CEL is sandboxed, cost-limited and typed, and many Kubernetes users already know it.

Any field in a template can be given as an expression by adding `Expr` to its name. The rendered object gets the field without the suffix, holding the expression's result, which can be any type. Giving both `image` and `imageExpr` is an error. A literal key that happens to end in `Expr`, for example in labels or a ConfigMap's data, would be taken for an expression too. That's rare, and it fails in the PreviewApp's checks rather than passing silently: either the value isn't valid CEL, or its result has the wrong type for the field, since labels and a ConfigMap's data only accept strings. Fields without a schema, such as a HelmRelease's `values`, are the exception. The workaround is to give the whole parent map as an expression, since the keys of an expression's result aren't checked for the suffix again.

The suffix needs a field name to attach to, which matters for lists. In a list of objects, like a container's `env`, each entry has fields of its own, so the suffix goes on those. A list of plain values, like a container's `args`, has no field names inside it, so the expression goes on the list itself and produces all of it:

```yaml
containers:
  - name: server
    imageExpr: '"ghcr.io/immich-app/immich-server:" + parameters.tag'
    argsExpr: '["--log-level", parameters.logLevel]'
    env:
      - name: PUBLIC_URL
        valueExpr: '"https://" + variables.host'
```

Expressions can refer to:

- **`parameters`**: the Preview's parameter values, with their declared types.
- **`preview`**: the preview's `name`, `namespace`, `labels` and `annotations`. Its `namespace` is the one its resources run in, not the one the Preview object lives in. Labels and annotations let automation pass in context, such as a pull request number.
- **`variables`**: see below.

Rendering is deterministic: expressions have no access to the time, randomness or anything else in the cluster, so the same PreviewApp and Preview always render the same objects.

kupra compiles and type-checks every expression when a PreviewApp changes. Problems are reported on the PreviewApp's status, together with the state checks of [ADR 2](0002-state.md) and the permission checks of [ADR 4](0004-namespaces-and-isolation.md), before any preview is affected.

The types an expression can use come from the parameters, the preview and the variables. The type its result must have comes from the schema of the field it fills in: the API server publishes schemas for every kind of resource, CRDs included. Fields without a schema, such as a HelmRelease's `values`, accept any type. Helm still validates a release's values against its chart's values schema, if the chart has one.

### Variables

`variables` gives names to values that several places need, so they're written once. Each has a `name` and an `expression`, and can use the parameters, the preview, and the variables listed before it, the same way variables work in Kubernetes' ValidatingAdmissionPolicy. Templates and links refer to them as `variables.<name>`:

```yaml
variables:
  - name: host
    expression: preview.name + ".immich.preview.example.com"
links:
  - name: Web
    urlExpr: '"https://" + variables.host'
```

### Links

Each of a PreviewApp's `links` has a `name` and a `urlExpr`. kupra's operator evaluates them for every preview and writes the results into the Preview's status.

### Per-resource settings

A few settings apply to a single resource in the template. They're given as annotations on it, and each holds a CEL expression that evaluates to true or false. Leaving one out means true.

- **`kupra.futo.tech/include`** decides whether the resource is rendered at all, for example `parameters.machineLearning`.
- **`kupra.futo.tech/state`** decides whether a recognised resource is handled as state, as described in ADR 2. `"false"` opts it out.

They stay on the applied objects, so it's visible why a resource is there and how it's treated.

### Applying and updating

kupra applies rendered resources with server-side apply, under its own field manager. Changes to a PreviewApp or to a Preview's parameters are rendered and applied to running previews straight away. A new image tag from a pull request, for example, simply rolls out. State is never restored again by an update: only a reset does that.

Resources that are no longer rendered, because they were removed from the template or their `include` became false, are deleted. State resources are the exception: kupra keeps them, and reports them on the Preview, until the preview is reset or deleted.

That's because the two kinds can't be lost the same way. A workload resource can always be recreated from the template. A state resource holds data the preview has built up since it started, which exists nowhere else: not in the template, and not in the snapshot it started from. Template changes reach every running preview at once, so a mistake like renaming a resource or flipping an `include` expression would otherwise delete that data everywhere. Deleting a preview's data is left to an explicit reset or deletion. Flux has an annotation to protect resources like volumes from its pruning for the same reason.

## Alternatives considered

- **Text templating, such as Go templates, Jinja or Helm.** More powerful, but string-based, with whitespace pitfalls. Its delimiters also collide with the templating layers PreviewApps often pass through on the way into a cluster, such as Flux's `${}` substitution, Helm and ResourceSets.
- **Marker objects,** such as `tag: {$expr: parameters.tag}`. These also work inside lists of plain values, but they're unusual and noisy for the common case.
- **Patches,** in the style of kustomize replacements. No new language, but verbose and brittle for anything beyond copying a value.
- **A full JSON schema for parameters.** Precise, but heavier than a creation form needs, and daunting for app authors.
- **Inferring the apply order from the expressions that refer to resources,** as kro does. With names fixed per namespace, there are few such references, and applying state first is the only ordering kupra needs.

## Consequences

- App authors use CEL, which is familiar from Kubernetes' own validation rules, but still new to some.
- Lists of plain values need an expression for the whole list.
- Template changes reach running previews immediately. State that drops out of a template stays until a reset.
- Templates can't use time or randomness.
