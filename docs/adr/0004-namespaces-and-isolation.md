# 4. Namespaces and isolation

- Status: Proposed
- Date: 2026-10-20

## Context

[ADR 1](0001-kupra.md) runs each preview in its own namespace, and treats both sides of a preview as untrusted. A PreviewApp's template can describe arbitrary resources, and a preview of a pull request runs code nobody has reviewed yet. This ADR decides where kupra's own resources live, how preview namespaces are named, and how kupra confines what a template produces and what a preview can reach.

## Decision

### Where things live

PreviewApps, their Previews and their PreviewSnapshots live together in an ordinary namespace, chosen by whoever sets up the app, for example `immich-previews`. That namespace is where people and automation work with kupra, and where access to an app's previews is granted.

Each Preview's resources run in a namespace of their own, which kupra creates when the Preview is created and deletes with it. Nothing else should create resources there: it belongs to the preview.

Keeping the Preview object outside the namespace it manages means creating a preview is a single object, and doesn't need permission to create namespaces.

### Naming

A preview's namespace is named `<namespace>-<preview>`, after the Preview's own namespace and name. So Preview `pr-1234` in `immich-previews` runs in `immich-previews-pr-1234`. If that's longer than Kubernetes allows, kupra shortens it and adds a short hash of the full name. The Preview's status records the namespace it actually uses.

kupra labels every preview namespace, so other tools can recognise and select them:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: immich-previews-pr-1234
  labels:
    kupra.futo.tech/app: immich
    kupra.futo.tech/preview: pr-1234
    kupra.futo.tech/preview-namespace: immich-previews
    pod-security.kubernetes.io/enforce: baseline
```

kupra never takes over a namespace it didn't create. If the name is already in use, the Preview reports the conflict.

### Confining templates

kupra renders a template only into its preview's namespace, and sets each resource's namespace itself.

kupra applies template resources with the permissions of a service account in the preview's namespace, not with its own. Admins decide what that account may do, through a role kupra binds to it there. A template can therefore only create the kinds of resources the admins allow, only in its own namespace, and can't grant itself more. Because the binding is namespaced, cluster-scoped resources like CRDs, ClusterRoles or webhooks are never allowed. Flux's multi-tenancy lockdown, which applies each tenant's resources as that tenant's service account, is the precedent.

The role is what enforces this, but finding out when a preview fails is too late. kupra also checks each PreviewApp's template against the role, and reports anything it wouldn't be allowed to create on the PreviewApp's status, before any preview uses it.

kupra's own work, like capturing and restoring state, uses kupra's own permissions. That work comes from kupra's code, not from the template.

### Network and resources

kupra manages these in every preview namespace, as set by the app's policy (below):

- **Network policies.** Outbound traffic is restricted. By default a preview can reach its own namespace, DNS, and destinations outside the cluster, but nothing else in the cluster. Because every preview is restricted this way, previews can't reach each other or the cluster's internals. Inbound traffic is left open, so the gateway, operators and monitoring can reach previews without kupra having to know where they run. Exactly how "outside the cluster" is expressed depends on the network plugin. Templates can't create their own network policies: network policies only ever add allowances, so one could undo the restrictions.
- **A resource quota and limit range,** so one preview can't starve the others.
- **Pod Security admission,** at the `baseline` level by default. That blocks privileged pods and host access, while still allowing images that run as root.

### Policies

The role templates are applied with, the network policies, the quota and the Pod Security level all come from a PreviewPolicy. It's a cluster-scoped resource, so only cluster admins can change it. kupra installs one named `default`. An admin can give an app different settings by labelling its namespace `kupra.futo.tech/policy: <name>`, the same way Pod Security admission is set per namespace. Other limits that belong to admins, such as how many previews may run at once, are expected to live here too.

### Shared infrastructure

Previews often rely on shared infrastructure: a Gateway to route to them, a ClusterSecretStore for credentials, a StorageClass. Templates can refer to these, but the infrastructure decides whether to accept a preview namespace, using its usual namespace selectors. A Gateway's `allowedRoutes`, for example, can admit routes only from namespaces labelled `kupra.futo.tech/app: immich`. kupra doesn't grant any access to shared infrastructure itself.

## Alternatives considered

- **One shared namespace for all previews,** as the current Immich setup does. That's simpler, but it leaves no boundary for network policy, quotas or access between previews. Every resource name also has to be made unique per preview.
- **Putting each Preview inside the namespace it manages.** Everything about a preview would then be in one place, but creating a preview would mean creating a namespace first. That's a chicken-and-egg problem for automation, and a privilege the dashboard shouldn't need.
- **A virtual cluster per preview,** such as vCluster. That gives stronger isolation and would allow cluster-scoped resources in templates, but it's much heavier than most previews need. It may be worth offering later for apps that need it.
- **Applying templates with kupra's own permissions.** Anyone who could write a PreviewApp would then effectively have kupra's permissions.

## Consequences

- Templates can't include cluster-scoped resources. Apps that need an operator rely on one installed in the cluster.
- Admins define what templates may create, and the network, quota and Pod Security settings, in PreviewPolicies. kupra ships a default one.
- Inbound traffic to previews is open within the cluster. Clusters that run other untrusted workloads should restrict it in their policy.
- Preview namespace names are derived, and occasionally shortened. The Preview's status is the source of truth.
- Shared infrastructure has to be configured to accept preview namespaces, which kupra's labels make straightforward.
