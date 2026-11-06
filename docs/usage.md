# Using kupra

This assumes kupra is [installed](install.md), and you can create Previews in an app's namespace that has a PreviewApp, such as the [Immich example](examples/immich.yaml).

## Start a preview

```yaml
apiVersion: kupra.futo.tech/v1alpha1
kind: Preview
metadata:
  name: pr-1234
  namespace: immich-previews
spec:
  appRef:
    name: immich
  parameters:
    tag: pr-1234
```

kupra creates a namespace for the preview, applies the app's template there, and reports progress in the Preview's status. `kubectl get previews` shows its phase and why it isn't ready yet, and `-o wide` adds its namespace. Its links, such as its URL, are in `status.links`.

The people who can change, reset and delete a preview are whoever created it, and anyone listed in `spec.owners`.

## Take a snapshot

A PreviewSnapshot captures a running preview's state: its PersistentVolumeClaims and CloudNativePG clusters.

```yaml
apiVersion: kupra.futo.tech/v1alpha1
kind: PreviewSnapshot
metadata:
  name: base-2026-11-06
  namespace: immich-previews
  labels:
    kupra.futo.tech/channel: base
spec:
  source:
    previewRef:
      name: pr-1234
```

Snapshotting a preview takes the `snapshot` permission on it, which its owners have if they can also create snapshots, and `kupra-admin` has for every preview. By default, a capture is `Offline`: kupra stops the preview's app while it captures, so the data is consistent, and starts it again afterwards. `mode: Online` keeps the app running. The snapshot is ready to use once `kubectl get previewsnapshots` shows it Ready. It outlives the preview it was taken from.

## Start from a snapshot

A preview's `restoreFrom` picks a snapshot by name with `snapshotRef`, or the newest ready one with matching labels with `snapshotSelector`:

```yaml
spec:
  restoreFrom:
    snapshotSelector:
      matchLabels:
        kupra.futo.tech/channel: base
```

A snapshot can't be deleted while a preview is being restored from it, or names it in `snapshotRef`. Its status says what's holding it.

## Reset a preview

Resetting puts a preview's state back to where it started: from the snapshot its `restoreFrom` picks now, or empty. Set the reset annotation to a new value, such as the time:

```sh
kubectl annotate preview pr-1234 --overwrite kupra.futo.tech/reset-requested-at="$(date +%s)"
```

## Delete a preview

Deleting a Preview deletes its namespace, with everything in it. Snapshots taken from it are kept.
