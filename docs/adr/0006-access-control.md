# 6. Access control

- Status: Proposed
- Date: 2026-10-20

## Context

Different people need different access to previews. Developers create and manage their own previews, reviewers look at them, app admins look after all of an app's previews, and cluster admins set policy. They reach kupra through the dashboard, kupra's CLI, kubectl and GitOps automation. [ADR 1](0001-kupra.md) leaves access control to Kubernetes RBAC, with OIDC for the dashboard. This ADR decides how identity works, how permissions are expressed, what owning a preview means, and who can get into a running preview.

## Decision

### One permission system

Every access decision is made by Kubernetes RBAC. The dashboard and kupra's CLI call the Kubernetes API with the user's own credentials, and hold no permissions of their own for acting on previews. Anything the dashboard can do, kubectl can do, and the other way round. Kubernetes' audit log records the actual person.

People sign in with an OIDC provider that the Kubernetes API server trusts. kupra works with any provider. Kubernetes' structured authentication configuration lets the API server trust several issuers alongside whatever authentication a cluster already uses, and map their claims to usernames and groups. Prefixing those names, like the `oidc:` below, keeps them apart from other identities in the cluster.

The dashboard signs people in as an OIDC client of the same provider, and passes their tokens on to the API server, which has to accept tokens issued to the dashboard. kupra itself never validates tokens: the API server does.

### Roles

kupra ships ClusterRoles meant to be bound in an app's namespace, so access is granted per app:

- **`kupra-viewer`** can see the app's PreviewApps, Previews and snapshots.
- **`kupra-creator`** can also create Previews and snapshots, and manage the Previews they own.
- **`kupra-admin`** can manage all of the app's Previews, snapshots and PreviewApps, and get into every running preview.

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
    name: oidc:immich-developers
```

PreviewPolicies are cluster-scoped, so they stay with cluster admins.

Creating snapshots is a trusted permission. A new snapshot can become the starting point for everyone's previews ([ADR 3](0003-snapshots.md)), so `kupra-creator` is for people trusted with an app's shared data. People who should only manage their own previews, such as pull request authors, get that through ownership instead.

### Owning a preview

A Preview's owners can change it, reset it, delete it, and get into it (see below). Owners who can also create snapshots can snapshot it.

Its owners are:

- **Whoever created it.** An admission policy records the creator in the `kupra.futo.tech/created-by` annotation. The value comes from the request itself, so it can't be forged or changed later.
- **Anyone listed in `spec.owners`**, as users or groups:

```yaml
spec:
  owners:
    - kind: Group
      name: github:octocat
```

Previews created by automation, such as a Flux ResourceSet, have no person as their creator. App admins manage them, and the automation can list owners. A pull request's author can own its preview once the identity provider puts their GitHub login in a claim that the API server maps to a group, like `github:octocat` above.

kupra enforces ownership with ordinary RBAC, so `kubectl auth can-i` gives the real answer. For each Preview, it creates a Role in the app's namespace covering just that Preview, and binds the owners to it.

Snapshots need one extra step, because RBAC controls who can create a PreviewSnapshot but not which preview it captures. The owners' Role also grants a custom `snapshot` verb on their Preview, and `kupra-admin` grants it on all of them. kupra checks that a snapshot's creator holds it, with a SubjectAccessReview, before capturing. Without that, anyone who can create snapshots could take an `Offline` snapshot of someone else's preview, and briefly take it down.

### Getting into a running preview

kupra binds a preview's owners, and the app's admins, to a role in the preview's own namespace. It allows reading pods and logs, exec and port-forwarding. `kubectl exec` and `kubectl logs` work directly, as the user. The dashboard's logs and terminal, and the database browser that's anticipated, use the same permission with the user's credentials.

App admins are whoever holds `kupra-admin` in the app's namespace. kupra carries those bindings into each preview namespace, much like the Hierarchical Namespace Controller propagates RBAC from a parent namespace to its children.

Who can open a preview's URL isn't decided by kupra. Routing lives in the template, so authentication in front of a preview, such as a gateway's OIDC support or oauth2-proxy, is set up there.

## Alternatives considered

- **The dashboard with its own permissions, checking access itself.** It would work without the API server trusting an identity provider. But it means two permission systems that can drift apart, and a web app holding broad permissions.
- **Checking ownership in admission policies**, by comparing the requester with a Preview's owners on every change. Fewer objects, but the permission logic would be hidden in expressions, and invisible to `kubectl auth can-i`.
- **Permissions per PreviewApp within a namespace,** for example a custom `use` verb on a PreviewApp that kupra checks when a Preview is created. The same pattern as snapshots would work, but apps mostly share a namespace in order to share snapshots, and putting apps that need different access in different namespaces covers the rest. It can be added later if needed.
- **Owners as a plain field, without recording the creator in admission.** Simpler, but kupra would have no reliable way of knowing who created a preview.

## Consequences

- Using the dashboard requires the API server to trust the identity provider. Many self-managed clusters and managed services can be configured that way, but not all. kubectl and GitOps work regardless.
- Recording the creator needs MutatingAdmissionPolicy, which is stable since Kubernetes 1.36.
- kupra maintains a few RBAC objects per preview.
- Access to a preview's URL is the template's concern, not kupra's.
