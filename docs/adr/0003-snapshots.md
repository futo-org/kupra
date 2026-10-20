# 3. Snapshots

- Status: Proposed
- Date: 2026-10-20

## Context

[ADR 2](0002-state.md) decides what counts as state, and how each kind of state is restored from and captured into a capture. This ADR decides how PreviewSnapshots work as a whole: how a consistent snapshot is taken, where its data lives, how it gets into a new preview's namespace, how a preview chooses one, and how long snapshots are kept.

One constraint shapes much of this. VolumeSnapshots are namespaced: a PVC can only be restored from a VolumeSnapshot in its own namespace, and CloudNativePG only recovers from snapshots in the cluster's namespace. Kubernetes has an alpha feature for cross-namespace data sources, but CloudNativePG can't use it yet.

## Decision

### Taking a snapshot

A PreviewSnapshot refers to the preview it captures. kupra captures each of that preview's state resources the way ADR 2 describes, and records what it captured from:

```yaml
apiVersion: kupra.futo.tech/v1alpha1
kind: PreviewSnapshot
metadata:
  name: immich-base-2026-10-20
  labels:
    kupra.futo.tech/channel: base
spec:
  source:
    previewRef:
      name: main
status:
  appRef:
    name: immich
  parameters:
    tag: commit-0123abc
  captures:
    - name: database
      group: postgresql.cnpg.io
      kind: Cluster
    - name: library
      group: ""
      kind: PersistentVolumeClaim
  conditions:
    - type: Ready
      status: "True"
```

Recording the app and its parameters lets people and tools spot obvious mismatches, such as restoring data from a newer release into an older one. kupra doesn't enforce compatibility, since what's compatible is up to each app.

A snapshot's captured data never changes. To refresh base data, take a new snapshot.

### Consistency

`spec.mode` chooses how a snapshot is kept consistent.

- **`Offline`** (the default): kupra stops the preview's app while it captures. It removes the resources that run the preview's pods, apart from state, captures every state resource, then applies them again. With nothing writing, the captures all reflect the same moment: a database row can't refer to a file that's missing from the volume capture. This costs the source preview a short outage. The app is applied again even if a capture fails.
- **`Online`**: kupra captures while the app keeps running. Each capture is consistent on its own, but captures are taken a moment apart, so data that spans them may not line up.

Further modes can follow, such as capturing several volumes atomically where storage supports group snapshots.

### Where snapshot data lives

Captures start out in the source preview's namespace, because that's where VolumeSnapshots and CloudNativePG backups are created. Once a capture is complete, kupra moves it next to the PreviewSnapshot: it binds the same underlying storage snapshot to a new VolumeSnapshot in the PreviewSnapshot's namespace, and carries over what's needed to restore it, such as CloudNativePG's backup annotations. Deleting the source preview, or its whole namespace, then leaves the snapshot intact. Deleting the PreviewSnapshot deletes the storage snapshots.

### Restoring into a preview

To restore, kupra binds the same storage snapshot to a VolumeSnapshot in the new preview's namespace, and points the state resource at it. These copies refer to the existing storage snapshot rather than duplicating it, so they cost no storage. They stay in the preview's namespace, so a reset from the same snapshot can reuse them. kupra removes them when the preview or their snapshot is deleted.

This needs storage that can import an existing snapshot into a new VolumeSnapshot. That's standard CSI behaviour, handled by the snapshot controller and the driver's ordinary restore, rather than something each driver opts into. A prototype with CloudNativePG 1.30 and the CSI hostpath driver confirmed the whole path: an online CloudNativePG backup, copied into another namespace and recovered there, with the source restorable again after the copy was deleted.

### Choosing a snapshot

A Preview's `restoreFrom` refers to a snapshot by name (`snapshotRef`) or selects one by label (`snapshotSelector`). A selector picks the newest ready snapshot that matches, and kupra records the choice in the Preview's status. Labels such as `kupra.futo.tech/channel: base` are only a convention: kupra gives them no special meaning.

Snapshots aren't tied to the PreviewApp they were taken from. Restoring matches captures to the preview's state resources by name and kind. A state resource with no matching capture starts empty, a capture that matches nothing is ignored, and both are reported on the Preview. This lets a variant of an app, such as a separate demo PreviewApp, start from the main app's snapshots, and lets a PreviewApp gain or drop state resources without invalidating every existing snapshot.

### Reset

Resetting a preview recreates it as if it had just been created: its state is restored again, and its workload starts fresh. A `snapshotSelector` is resolved again, so a reset picks up the newest matching snapshot. To always return to the same data, use a `snapshotRef`.

A reset is requested by setting the `kupra.futo.tech/reset-requested-at` annotation on the Preview to the current time, and kupra records the last value it handled in the Preview's status. This follows Flux's `reconcile.fluxcd.io/requestedAt`, so a dashboard button, a CronJob or a GitOps change can all request a reset without owning the Preview's spec.

### Retention and deletion

A PreviewSnapshot can set a `spec.ttl`, after which kupra deletes it. That suits one-off snapshots, such as one taken to share a bug's state. Base snapshots are usually pruned by count rather than age, keeping the newest few in a channel. That kind of retention is anticipated alongside scheduled snapshots, and until then it's left to whatever takes the snapshots.

A PreviewSnapshot isn't removed while it's still needed: while a restore from it is in progress, or while a Preview's `snapshotRef` names it. Until then, deleting it leaves it pending, the way Kubernetes keeps a PVC that a pod still uses. Its status says what's holding it.

## Alternatives considered

- **Leaving captures in the source preview's namespace.** They would disappear with the preview, and restoring elsewhere needs copies anyway.
- **Cross-namespace volume data sources.** Still alpha, and CloudNativePG can't use them yet.
- **Object store backups for CloudNativePG.** These can be restored in any namespace without copying snapshots around, but every restore copies all the data instead of using the storage's snapshots. This is worth revisiting with portable snapshots.
- **Online by default.** It avoids the outage, but makes inconsistent snapshots the default for something that's meant to seed many previews.
- **Pinning a reset to the snapshot a preview first used.** That would be predictable, but a `snapshotRef` already does it, and a reset that picks up the latest base data is more useful for development previews.
- **Requiring every state resource to have a capture.** Stricter, but every snapshot would break as soon as a PreviewApp gained a state resource, and app variants couldn't share snapshots.

## Consequences

- kupra needs storage that supports volume snapshots, and importing existing snapshots into new VolumeSnapshots.
- Taking an `Offline` snapshot briefly takes the source preview's app down.
- Snapshots live in the cluster's own storage. Moving them between clusters, or out of the cluster, is for portable snapshots.
- Deleting a snapshot can be held up while it's in use.
- A typo in a state resource's name doesn't fail a restore: that resource starts empty, with a warning on the Preview.
