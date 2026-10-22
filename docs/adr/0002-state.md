# 2. State

- Status: Proposed
- Date: 2026-10-19

## Context

[ADR 1](0001-kupra.md) makes state kupra's core concern. kupra recognises some resources in a PreviewApp's template as state, starts them from a snapshot, and captures them into new snapshots. This ADR decides what that means: what counts as state, what supporting a kind of state involves, and how that support is built and extended. How snapshots are taken and moved between namespaces is the subject of a separate ADR.

## Decision

### What counts as state

A resource in a template is state if kupra has support for its kind. Support is opt-out: a recognised resource is treated as state unless it's annotated otherwise. Everything else in the template is workload: applied as-is, and free to be deleted and recreated, for example while a preview sleeps.

The first version supports:

- **PersistentVolumeClaims**, including the claims a StatefulSet creates from its `volumeClaimTemplates`.
- **CloudNativePG clusters.**

Anticipated in the future, roughly in this order:

- **Generated secrets.** These are values like encryption keys, generated once, that have to travel with the data they protect. Outline encrypts database columns with its `SECRET_KEY`, and Zulip hashes a salt into every upload path. kupra would generate these itself, because "generated once, then inherited by every preview restored from this data" is a rule about snapshots that no external secrets tool can know. Secrets from an external store stay ordinary template resources.
- **Object storage buckets**, through the Kubernetes Container Object Storage Interface (COSI).
- **Other database operators**, such as those for MySQL, MariaDB or MongoDB.
- **Search engines** like OpenSearch and Elasticsearch, through their own snapshot APIs.
- **State created outside the template**, such as a volume a Helm chart or another operator creates, declared explicitly. Until then kupra can't see such state: it isn't captured, and previews start without it.

### What supporting a kind involves

A PreviewSnapshot holds one capture of each state resource in the preview, matched to the resource by name. For Immich, that's a capture of the `database` cluster and one of the `library` PVC. When a new preview starts from the snapshot, its `database` cluster is restored from the `database` capture.

Support for a kind of state answers four questions:

- **Recognise:** which resources does it handle?
- **Check:** can it handle this resource as written? Problems are reported on the PreviewApp, before any preview uses it. CNPG support, for example, refuses a cluster that configures its own backups or WAL archiving.
- **Restore:** how does a resource start from a capture? For a PVC, that's a `dataSourceRef`. For a CNPG cluster, it's a recovery bootstrap.
- **Capture:** how is a capture taken? For a PVC, that's a VolumeSnapshot. For a CNPG cluster, it's a CloudNativePG `Backup`.

kupra creates state resources before the rest of the template, so restored data and pre-created claims are in place before anything uses them.

More capabilities are anticipated, optional per kind: hibernating while a preview sleeps (CNPG has a hibernation mode), and exporting to and importing from portable formats.

### Work through each tool's own interface

kupra captures and restores each resource the way the tool that manages it intends. A plain PVC is managed by nothing but Kubernetes itself, so kupra uses the standard VolumeSnapshot API. A CNPG cluster's volumes belong to CNPG, so kupra asks CNPG for a `Backup` and restores with CNPG's recovery bootstrap, rather than snapshotting those volumes itself. Operators maintain their own invariants, and their backup paths handle consistency details that a snapshot taken behind their back would miss.

Supporting a kind also means owning the parts of the resource that capture and restore depend on. For CNPG clusters, that's `bootstrap`. A template can set `bootstrap.initdb` options that shape a new cluster, such as its locale. kupra uses them when a preview starts empty, and replaces them with a recovery bootstrap when it starts from a snapshot. Apps use CNPG's default database, owner and generated credentials, which stay valid across a restore. Renaming them fails the check.

### How support is built

Support for each kind is written as pure functions. Given the rendered resource and what currently exists in the cluster, it returns the objects it wants to exist (a `Backup` to create, fields to set) and whether it's done. One engine in kupra applies those objects and watches the results, and only the engine talks to the Kubernetes API.

That keeps each kind easy to test, makes kupra safe to restart halfway through a capture (progress lives in cluster objects, not in memory), and puts a boundary between kupra and each kind that's just data going in and out.

Support is built into kupra for now. Because the boundary is a plain request and response, it could later be opened up to implementations outside kupra, the way CloudNativePG's plugin interface and Crossplane's composition functions work. We'll do that when there's a real need.

### Opting out

```yaml
metadata:
  annotations:
    kupra.futo.tech/state: "false"
```

An opted-out resource is ordinary workload. It isn't captured or restored, kupra leaves it as written, and it may be deleted and recreated, losing its data. This is meant for:

- caches
- data the app regenerates or seeds itself on startup
- resources the check would refuse, such as a CNPG cluster with its own backups, or one imported from a sanitised source

Support is opt-out rather than opt-in because the two ways of getting it wrong aren't equal. A forgotten opt-out wastes snapshot space on a cache. A forgotten opt-in silently leaves data out of snapshots.

## Alternatives considered

- **A typed state section in PreviewApp**, such as `state: [{cnpg: {...}}]`. kupra would have to re-expose each tool's API, and authors would describe their database twice.
- **Snapshotting volumes underneath operators.** Generic, but it bypasses the operator's own consistency handling and invariants, and only works for state that lives in volumes.
- **Declarative state definitions:** a resource describing how to restore and capture a kind with templates and expressions, so new kinds need no code. Attractive, but real kinds need logic, like rewriting a bootstrap or capturing in several steps. Worth revisiting once several kinds exist and their patterns are clear.
- **Plugins from the start.** There's no second implementer yet, and the request/response boundary keeps the option open.
- **A CloudNativePG plugin.** CloudNativePG's plugin interface (CNPG-I) would let kupra take part in CNPG's own backups and recoveries. CNPG's built-in volume snapshot backups cover the first version without one. It's worth revisiting with portable snapshots, where a plugin could let CNPG write to and recover from kupra's portable format directly.
- **Opt-in instead of opt-out.** See above.

## Consequences

- Authors get state handling from plain manifests, with no kupra-specific configuration in the common case.
- Each new kind of state is code in kupra, and what kupra can do with a kind is limited by what its tool exposes.
- Templates must leave the parts kupra owns, like a CNPG cluster's bootstrap, to kupra, or opt the resource out.
- State created outside the template isn't captured until it can be declared.
- Opted-out resources lose their data whenever their workload is recreated.
