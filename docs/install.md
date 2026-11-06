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

CloudNativePG's pods talk to the Kubernetes API server. The default PreviewPolicy's `restrict-egress` NetworkPolicy only lets previews reach public addresses, so on a cluster that enforces NetworkPolicies and serves the API on a private address, CloudNativePG clusters in previews never become ready. Allow their pods to reach the API server with another NetworkPolicy. Find its addresses and port with:

```sh
kubectl get endpointslices --namespace default --selector kubernetes.io/service-name=kubernetes
```

Lists in values replace the chart's, so set both the chart's policy and the new one, with the API server's address and port:

```yaml
defaultPolicy:
  networkPolicies:
    - metadata:
        name: restrict-egress
      spec:
        podSelector: {}
        policyTypes: [Egress]
        egress:
          - to:
              - podSelector: {}
          - to:
              - namespaceSelector:
                  matchLabels:
                    kubernetes.io/metadata.name: kube-system
                podSelector:
                  matchLabels:
                    k8s-app: kube-dns
          - to:
              - ipBlock:
                  cidr: 0.0.0.0/0
                  except:
                    - 10.0.0.0/8
                    - 172.16.0.0/12
                    - 192.168.0.0/16
                    - 100.64.0.0/10
                    - 169.254.0.0/16
              - ipBlock:
                  cidr: ::/0
                  except:
                    - fc00::/7
                    - fe80::/10
    - metadata:
        name: allow-cnpg-api-server
      spec:
        podSelector:
          matchExpressions:
            - key: cnpg.io/cluster
              operator: Exists
        policyTypes: [Egress]
        egress:
          - to:
              - ipBlock:
                  cidr: 172.18.0.2/32
            ports:
              - protocol: TCP
                port: 6443
```
