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

CloudNativePG's pods talk to the Kubernetes API server, which the default `restrict-egress` NetworkPolicy blocks when it's on a private address. The default PreviewPolicy's `allow-cnpg-api-server` NetworkPolicy lets them reach any address on TCP ports 443 and 6443, where API servers usually listen. If yours listens on another port, change that policy's ports. Find the port with:

```sh
kubectl get endpointslices --namespace default --selector kubernetes.io/service-name=kubernetes
```
