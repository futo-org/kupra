# Installing kupra

kupra needs Kubernetes 1.36 or newer. To snapshot state, it needs the CSI snapshot controller, and a default VolumeSnapshotClass for each CSI driver previews use. Previews with CloudNativePG clusters need CloudNativePG, which kupra is tested with at version 1.30.

## Install

```sh
helm install kupra oci://ghcr.io/futo-org/charts/kupra --version <version> \
  --namespace kupra-system --create-namespace
```

## Upgrade

Helm installs the chart's CRDs, but doesn't upgrade them. Apply them first:

```sh
helm show crds oci://ghcr.io/futo-org/charts/kupra --version <version> | kubectl apply --server-side -f -
helm upgrade kupra oci://ghcr.io/futo-org/charts/kupra --version <version> --namespace kupra-system
```

With Flux, set `crds: CreateReplace` under the HelmRelease's `install` and `upgrade` instead.

## The default PreviewPolicy

The chart creates the `default` PreviewPolicy from its `defaultPolicy` value. Change the defaults there, not by editing the object. Helm merges maps in values with the chart's, replaces lists, and removes fields set to `null`:

```yaml
defaultPolicy:
  resourceQuota:
    hard:
      requests.cpu: "8"
      requests.memory: null
```

## CloudNativePG

CloudNativePG's pods talk to the Kubernetes API server, which the default `restrict-egress` NetworkPolicy blocks when it's on a private address. NetworkPolicies can only allow the API server by address, so the default PreviewPolicy's `apiServerAccess` has kupra do it: it creates the NetworkPolicy `kupra-api-server` in each preview namespace, which lets pods with a `cnpg.io/cluster` label reach the addresses and ports of the `kubernetes` Service's endpoints, and keeps it up to date.
