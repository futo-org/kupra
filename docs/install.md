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

## Other policies

An app namespace uses the `default` PreviewPolicy unless it's labelled with another one's name. Fields a policy leaves out are taken from `default`:

```sh
kubectl label namespace immich-previews kupra.futo.tech/policy=immich
```

## What templates may contain

Templates are applied with the policy's ClusterRole, `kupra-preview-template` by default, which allows common workload kinds, CloudNativePG clusters and Gateway API routes. A PreviewApp that uses another kind fails its checks until the role allows it. Add rules to the role with a ClusterRole carrying its aggregation label:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kupra-preview-template-external-secrets
  labels:
    kupra.futo.tech/aggregate-to-template: "true"
rules:
  - apiGroups: [external-secrets.io]
    resources: [externalsecrets]
    verbs: [get, list, watch, create, update, patch, delete]
```

A policy can also name a ClusterRole of its own in `apply.clusterRoleRef`. kupra can only bind roles an admin has approved for it, by labelling them `kupra.futo.tech/aggregate-to-operator: "true"`.

## Giving people access

kupra's roles are bound in an app's namespace ([ADR 6](adr/0006-access-control.md)):

- `kupra-viewer` can see the app's PreviewApps, Previews and PreviewSnapshots.
- `kupra-creator` can also create Previews and PreviewSnapshots, and manage the Previews they own.
- `kupra-admin` can manage everything in the namespace, and get into every running preview.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: immich-developers
  namespace: immich-previews
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kupra-creator
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: immich-developers
```

[Using kupra](usage.md) covers what they can do from there.
