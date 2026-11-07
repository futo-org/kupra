# Installing kupra

kupra needs Kubernetes 1.36 or newer. To snapshot state, it needs the CSI snapshot controller, and a default VolumeSnapshotClass for each CSI driver previews use. Previews with CloudNativePG clusters need CloudNativePG, which kupra is tested with at version 1.30.

## Install

```sh
helm install kupra oci://ghcr.io/futo-org/charts/kupra --version <version> \
  --namespace kupra-system --create-namespace
```

## Upgrade

Helm installs the chart's CRDs, but doesn't upgrade them. Apply them first, taking them over from Helm:

```sh
helm show crds oci://ghcr.io/futo-org/charts/kupra --version <version> | kubectl apply --server-side --force-conflicts -f -
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

## Dashboard

The dashboard signs people in with an OIDC provider, and acts as them, so the API server has to trust that provider and accept tokens issued to the dashboard's client, for example with [structured authentication configuration](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#using-authentication-configuration). People need kupra's roles in the apps' namespaces ([ADR 6](adr/0006-access-control.md)).

Register the dashboard with the provider, with `<publicURL>/auth/callback` as its redirect URL. Then create a Secret with the client's secret. The dashboard encrypts its session cookies with keys derived from it, so rotating it signs everyone out:

```sh
kubectl -n kupra-system create secret generic kupra-dashboard --from-literal=client-secret=<secret>
```


The issuer URL and client ID can go in the Secret too, as `issuer-url` and `client-id`, instead of the values below.

```yaml
dashboard:
  enabled: true
  publicURL: https://previews.example.com
  oidc:
    issuerURL: https://id.example.com
    clientID: kupra
  existingSecret: kupra-dashboard
```

The chart creates the `kupra-dashboard` Service. Route to it with your own Ingress or HTTPRoute.
