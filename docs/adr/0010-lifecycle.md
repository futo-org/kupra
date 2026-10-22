# 10. Lifecycle

- Status: Proposed
- Date: 2026-10-22

## Context

ADRs 2, 3 and 8 describe what kupra does to a preview: restore its state, apply its template, capture it, reset it and delete it. Each was described on its own. This ADR decides how they fit together: how kupra decides what to do next, what happens when several things are asked for at once, how progress is reported, and when a preview counts as ready.

## Decision

### How kupra decides what to do

Like any Kubernetes controller, kupra is level-triggered. Each time it looks at a Preview or a PreviewSnapshot, it compares what's asked for with what currently exists in the cluster, and decides the next step from that alone. It keeps no memory between runs. Anything that has to carry over is written into cluster objects, so a missed event or a restart of kupra changes nothing.

So the lifecycle isn't a stored state with transitions. It's a set of rules, applied to what kupra observes, in order of precedence. Phases are derived from the same observations and are only for display: kupra never reads a phase back to decide what to do.

### Where requests are recorded

| Request | Recorded as |
| --- | --- |
| Update | The Preview's `generation` is ahead of its `status.observedGeneration`. |
| Reset | `kupra.futo.tech/reset-requested-at` differs from `status.lastHandledResetAt`. |
| Snapshot | A PreviewSnapshot refers to the preview, and hasn't succeeded or failed yet. |
| Deletion | The Preview has a deletion timestamp, and kupra's finalizer holds it until cleanup is done. |

### Precedence

For a Preview, the first rule that applies decides what kupra does:

1. **It's being deleted.** A capture of it that's already running finishes first. Then kupra takes the final snapshot, if one is configured, and removes everything.
2. **A capture of it is running.** Changes to the template and parameters are held back, so the snapshot matches the parameters it records. For an `Offline` or `ScaleDown` capture, the app also stays stopped.
3. **A reset is requested.** kupra stops the app, removes the state, restores it again, and records the reset as handled. Captures that haven't started yet wait until after the reset. Otherwise a capture waiting for a broken preview to become ready would block the reset that's meant to fix it.
4. **Otherwise,** kupra converges to the spec. It creates and restores any missing state, waits for state to be ready, then applies the rest of the template.

For a PreviewSnapshot:

1. **It's being deleted.** It's held while it's in use, as [ADR 3](0003-snapshots.md) describes.
2. **It hasn't started.** It starts once its source preview is running, with no other capture of it running and no reset waiting. Captures of the same preview start in the order they were created.
3. **It's running.** kupra continues the captures, then moves them next to the snapshot.

### Phases

Phase names follow KubeVirt, whose VirtualMachines have a similar life, and Pods where the two overlap. A Preview's phase is one of:

| Phase | When |
| --- | --- |
| `Pending` | It can't start yet, for example because its PreviewApp's checks fail or its namespace name is taken. |
| `Provisioning` | Its state resources are being created or restored, or aren't ready yet. |
| `Starting` | Its state is ready, but the rest of the template isn't, for example while it first starts or while an update rolls out. |
| `Running` | Everything is ready. |
| `Stopping` | Its app is being stopped, for a capture or a reset. |
| `Stopped` | Its app is stopped, for example during an `Offline` capture. |
| `Terminating` | It's being deleted. |
| `Failed` | Something went wrong that retrying won't fix, such as a missing snapshot. A reset or deletion still works. |

The phase describes the preview, not what kupra is doing to it. An `Offline` capture shows as `Running`, `Stopping`, `Stopped`, `Starting`, then `Running` again. A reset shows as `Running`, `Stopping`, `Provisioning`, `Starting`, `Running`. The reason is in the `Reconciling` condition, and a capture's progress is on its PreviewSnapshot.

A PreviewSnapshot's phase is `InProgress`, `Succeeded`, `Failed` or `Deleting`, alongside a `readyToUse` field, the same as KubeVirt's VirtualMachineSnapshot. While it waits for its turn, it's `InProgress`, and the reason says it's waiting. If a capture fails, the source preview's app is started again regardless.

Tools should rely on the conditions rather than the phase: `Ready`, `Reconciling` and `Stalled`, as [ADR 9](0009-implementation.md) describes.

### When state and previews are ready

kupra waits for state to be ready before applying the rest of the template, so an app doesn't start against a database that's still recovering. Each kind of state decides what ready means. For a CloudNativePG cluster, it's the cluster's own `Ready` condition. For a PVC it's only that the claim exists. With storage that binds a volume only once a pod uses it, waiting for more would wait forever.

A preview is ready when every resource kupra applied is ready, judged by the same rules Flux and `kubectl wait` use. Readiness probes in the template are what make that meaningful. If a preview hasn't become ready within a time its PreviewApp can set, 30 minutes by default, kupra marks it `Stalled` but keeps trying.

### A final snapshot on deletion

A Preview can ask for a snapshot to be taken when it's deleted:

```yaml
spec:
  onDelete:
    snapshot:
      ttl: 168h
```

kupra takes it itself, as part of deleting the preview, so it doesn't depend on who or what deleted it. A pull request preview removed by automation keeps its state for a week, for example. The capture is `Offline`, since the app is going away anyway. The snapshot is named after the preview, carries no channel labels, and expires after its `ttl`. If the namespace the Preview lives in is itself being deleted, there's nowhere to keep the snapshot: Kubernetes refuses new objects there, and would delete them along with the namespace anyway. kupra then skips the final snapshot and reports that it did.

## Alternatives considered

- **An explicit state machine,** with the current state stored and transitions driven by events. It's easier to draw, but it goes wrong as soon as an event is missed or kupra restarts partway through. Level-triggered rules are how Kubernetes controllers stay correct.
- **Phases named after operations,** such as `Snapshotting` or `Resetting`. They describe what kupra is doing rather than the state of the preview, and they'd multiply with every new feature. The `Reconciling` condition's reason covers that.
- **Running operations in parallel,** with finer-grained locking. More complex, and little use for previews, where operations are rare and mostly short.
- **Rejecting conflicting requests instead of letting them wait.** Simpler for kupra, but every client, the dashboard and GitOps automation alike, would need its own retry logic.
- **Waiting for every state resource to be fully bound.** It deadlocks with storage that binds volumes only when a pod first uses them.
- **Conditions only, without a phase.** That's closer to Kubernetes' API conventions, but harder for people to read at a glance. Conditions stay the contract for tools.

## Consequences

- Requests can wait behind each other. A long capture delays a reset or an update.
- A preview with a final snapshot takes a little longer to delete.
- Previews only report ready as accurately as their templates' readiness probes allow.
