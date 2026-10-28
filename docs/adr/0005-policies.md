# 5. Policies

- Status: Accepted
- Date: 2026-10-20

## Context

[ADR 4](0004-namespaces-and-isolation.md) confines every preview: templates are applied with a limited role, and kupra manages network policies, a quota and Pod Security admission in each preview namespace. It puts those settings in a cluster-scoped PreviewPolicy, chosen per app by a namespace label. This ADR decides what a PreviewPolicy contains, how an app gets one, and what happens when it changes.

## Decision

### What a policy contains

A PreviewPolicy holds everything an admin controls about the previews it applies to:

```yaml
apiVersion: kupra.futo.tech/v1alpha1
kind: PreviewPolicy
metadata:
  name: default
spec:
  apply:
    clusterRoleRef:
      name: kupra-preview-template
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
  resourceQuota:
    hard:
      requests.cpu: "4"
      requests.memory: 16Gi
  limitRange:
    limits:
      - type: Container
        defaultRequest:
          cpu: 100m
          memory: 128Mi
        default:
          memory: 1Gi
  namespaceMetadata:
    labels:
      pod-security.kubernetes.io/enforce: baseline
      example.com/gateway-access: previews
```

- **`apply.clusterRoleRef`** names the ClusterRole that templates are applied with. kupra binds it, within each preview namespace, to the account it applies templates as.
- **`networkPolicies`**, **`resourceQuota`** and **`limitRange`** are created in each preview namespace as written. A quota counts what pods request, and is checked when pods are created. It doesn't reserve capacity or cap actual usage, which is what the limit range's default limits are for.
- **`namespaceMetadata`** adds labels and annotations to preview namespaces. That covers Pod Security admission, which is configured by namespace labels, and labels that shared infrastructure or other tools select namespaces by.

kupra's chart installs the `default` policy with every setting spelled out, like the example above, and admins change the defaults through the chart's values. Other policies fall back to `default` for any field they leave out, one level deep. Labels and annotations are merged, with the policy's own over `default`'s, so adding an unrelated label doesn't drop Pod Security's `enforce: baseline`. Loosening a setting, for example with an empty list of network policies, takes an explicit choice.

Limits that belong to admins, such as how many previews may run at once, are anticipated as further fields.

### The role templates are applied with

kupra ships a ClusterRole for templates that covers common workloads: Deployments, StatefulSets, Jobs, Services, ConfigMaps, Secrets, PersistentVolumeClaims, Gateway API routes, and the kinds of state kupra supports. It aggregates any ClusterRole labelled `kupra.futo.tech/aggregate-to-template: "true"`, the way Kubernetes' built-in `edit` and `admin` roles do. So an admin can allow, say, ExternalSecrets or an operator's resources for every app by adding one small ClusterRole, or point a single app's policy at a different role altogether.

kupra can only bind the roles it has been allowed to. It may bind its own template role. An admin who points a policy at a different role must also allow kupra to bind it, with a ClusterRole labelled `kupra.futo.tech/aggregate-to-operator: "true"` that grants `bind` on it. So kupra can never hand out other roles, such as `cluster-admin`, even through a bug. The PreviewApp's checks report a policy role that kupra isn't allowed to bind.

### How an app gets a policy

An app's namespace selects its policy with the `kupra.futo.tech/policy` label. Without the label, the policy named `default` applies. Each app namespace has exactly one policy, so there are no precedence rules to reason about. Namespaces are cluster-scoped, so by default only cluster admins can change the label. That's the same model as Pod Security admission's namespace labels.

If the label names a policy that doesn't exist, kupra refuses to create previews in that namespace and reports why. It doesn't fall back to `default`, which might be more permissive than intended.

### Changing a policy

kupra applies policy changes to existing previews straight away: it updates the role binding, network policies, quota, limit range and labels in every affected preview namespace, and checks the affected PreviewApps' templates against the role again. From there, Kubernetes' own rules decide what happens to running pods. A tightened Pod Security level or quota, for example, doesn't stop running pods, but does stop new ones that don't comply.

## Alternatives considered

- **Settings in kupra's own configuration.** Simpler, but global only, and not visible or auditable as resources in the cluster.
- **Namespaced policies in the app's namespace.** Whoever controls that namespace could loosen their own confinement.
- **Policies that select namespaces.** Overlapping selectors need precedence rules, which makes it hard to see which policy applies.
- **PreviewApps naming their own policy, gated by permission to use it.** This is roughly how PodSecurityPolicy worked, and that was removed from Kubernetes partly because it was hard to tell which policy applied to what.
- **Defaults built into kupra's code.** Admins couldn't see them as a resource, or change them.
- **Policies that inherit from each other in chains.** More flexible, but harder to read. Falling back to `default`, one level deep, covers the common case.
- **Applying templates with Kubernetes' built-in `edit` role.** It already covers most workloads, and many operators extend it. But it includes NetworkPolicies, which templates mustn't create, and what it contains is decided by every operator installed in the cluster, not by kupra's admins.
- **Leaving it to a policy engine.** Kyverno, for example, can generate network policies and quotas for namespaces with a given label, and kupra's labels make that possible on top. But secure defaults shouldn't depend on installing one, and the role templates are applied with is kupra's own concern.

## Consequences

- Admins get one resource per kind of app that describes everything they control about its previews.
- Apps that need more than the default role for templates, for example ExternalSecrets or an operator's resources, need an admin to allow them.
- Policy changes reach running previews immediately, within Kubernetes' own rules.
- A namespace that names a missing policy can't create previews until that's fixed.
