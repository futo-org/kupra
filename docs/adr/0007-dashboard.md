# 7. Dashboard

- Status: Proposed
- Date: 2026-10-20

## Context

[ADR 1](0001-kupra.md) puts a basic dashboard in the first version: for people who don't use Kubernetes, it's much of what kupra offers over a hand-built setup. [ADR 6](0006-access-control.md) decides that the dashboard acts with each user's own credentials. This ADR decides what the dashboard does, and how it's built into the system.

## Decision

### What it does

The first version covers the everyday flow:

- **Browse.** The apps a user can see, their previews and each preview's status, links, parameters, owners, the snapshot it started from, and where it came from, such as a pull request.
- **Create.** A form generated from the PreviewApp's `parameters`: types become widgets, enums become choices, and `uiHint` asks for special pickers, like a list of an image's tags. The user also picks a snapshot to start from, by channel or by name.
- **Act.** Reset, snapshot and delete a preview. Browse and delete snapshots, and see what's holding one that can't be deleted yet.

Anticipated later: logs, a terminal, the database browser, sleeping and waking previews, cloning, and downloading snapshots.

The dashboard only shows what a user is allowed to do. It asks the API which actions the user may take, and hides or disables the rest.

### Nothing that only the dashboard knows

The dashboard keeps no state of its own. Everything it shows comes from kupra's resources, and every action it takes is an ordinary change to them. So anything the dashboard can do, GitOps and kubectl can do too.

That also means anything clients would otherwise have to compute is worked out by kupra's operator. For example, the operator evaluates a PreviewApp's `links` for each preview and writes the result into the Preview's status, where the dashboard, the CLI and any other tool read the same URLs:

```yaml
status:
  namespace: immich-previews-pr-1234-7f3a
  links:
    - name: Web
      url: https://pr-1234.immich.preview.example.com
```

### Previews managed by automation

Some previews are created by automation, such as a Flux ResourceSet. Editing or deleting those from the dashboard would only be undone by the next reconciliation. The dashboard recognises previews managed by another tool, shows what manages them, and disables changes that tool would revert. Actions that don't touch the Preview's spec, like resetting or snapshotting, still work.

### A separate, unprivileged component

The dashboard is its own deployment, released together with kupra's operator but optional to install. Everything it shows about previews, and everything it does, uses the user's credentials.

The one exception is finding the apps a user can see. Kubernetes can't tell someone which namespaces they have access to when their grants are all namespaced. Granting everyone a cluster-wide read of PreviewApps would expose every app's template to every user, since RBAC has no "list names only" permission. Instead, kupra's operator labels the namespaces that contain PreviewApps, and the dashboard's only permission of its own is listing namespaces. It then checks, with the user's credentials, which of those apps the user can see, and shows only those.

The operator, on the other hand, holds broad permissions: it creates namespaces and applies templates as other accounts. Keeping the web-facing part out of the operator keeps those permissions away from the most exposed surface.

### Signing in and talking to the cluster

The dashboard is served by a small backend, which is the only thing the browser talks to. The backend signs people in, and passes their requests on to the Kubernetes API with their token. So the API server doesn't need to be reachable from browsers.

The token stays out of the page itself: the browser holds only HttpOnly cookies, with the access and refresh tokens encrypted inside them. That keeps the backend stateless, with no session store to run. Tokens can exceed a single cookie's size limit, so they're split across several cookies when needed, the way oauth2-proxy does it. Logging out revokes the refresh token with the identity provider, so a stolen cookie is only useful until its short-lived access token expires. These tokens carry the user's full cluster permissions, including exec into their previews. If the page held them, one cross-site scripting bug would let an attacker take a token and use it from anywhere until it expires, refresh token included. This is the backend-for-frontend pattern that the OAuth working group recommends for browser apps.

The backend also watches kupra's resources as the user, and pushes changes to the page, so status updates appear live.

## Alternatives considered

- **Building the dashboard into the operator.** One less deployment, but it puts a web-facing surface next to the operator's permissions.
- **A browser app that talks to the Kubernetes API directly.** No backend needed, but the API server would have to be reachable from browsers, and tokens would live in the page.
- **A thin proxy, with tokens kept in the browser.** The API server stays private, but the page still holds tokens with the user's full cluster permissions.
- **Discovering apps with the user's own credentials,** by granting everyone who signs in a cluster-wide read of PreviewApps. The dashboard would then hold no permissions at all, but every user could read every app's template.
- **A plugin for an existing Kubernetes UI,** such as Headlamp, or a developer portal like Backstage. That's less to build, but these tools are aimed at people who already know Kubernetes or already run that portal. kupra's dashboard is meant for people who just want a preview. Plugins could still be added alongside it later.

## Consequences

- One more component to deploy, though it's optional. GitOps-only setups can leave it out.
- Each person sees only what their own permissions allow.
- The dashboard can only offer what kupra's resources express. Every feature has to exist in the API first.
