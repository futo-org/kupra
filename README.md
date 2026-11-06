# kupra

Preview environments for Kubernetes, with first-class state.

A preview is a running copy of an app, often for a pull request, in its own namespace. kupra treats the app's data as part of the preview. A preview can start from a snapshot of another preview's databases and volumes instead of an empty app. It can be reset to where it started, and its state can be captured as a new snapshot for others to start from.

- **PreviewApp** describes an app: the manifests a preview runs, with parameters and [CEL](https://cel.dev) expressions filled in per preview.
- **Preview** is one running instance of a PreviewApp, in its own namespace.
- **PreviewSnapshot** is a captured copy of a preview's state. kupra supports PersistentVolumeClaims, through VolumeSnapshots, and [CloudNativePG](https://cloudnative-pg.io) clusters, through CloudNativePG's volume snapshot backups.
- **PreviewPolicy** holds the admin's settings for preview namespaces: the role templates are applied with, network policies, quotas and limits.

```yaml
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
```

kupra's API is `v1alpha1`, and may still change between releases.

## Documentation

- [Installing kupra](docs/install.md)
- [Using kupra](docs/usage.md)
- [Examples](docs/examples): Immich, and Outline and Zulip, which use features planned for later
- [Architecture decision records](docs/adr), starting with [ADR 1](docs/adr/0001-kupra.md)
- [Contributing](CONTRIBUTING.md)

## License

[Apache 2.0](LICENSE)
