<script lang="ts">
  import { Alert, Card, CardHeader, CardTitle, Heading, LoadingSpinner, Text } from '@immich/ui';
  import { resolve } from '$app/paths';
  import { getAppNamespaces } from '#lib/api/index.ts';
  import ReadyBadge from '#lib/components/ReadyBadge.svelte';
  import { groupApps } from '#lib/kube/apps.ts';
  import { pollValue } from '#lib/kube/poll.ts';
  import { pollLists } from '#lib/kube/watch.svelte.ts';

  let namespaces = $state<string[]>([]);
  let namespacesError = $state<Error>();
  let namespacesLoaded = $state(false);

  // Namespaces are looked for again as often as their apps are listed, so new ones show up. They're only replaced when
  // they change, as that lists their apps again.
  $effect(() =>
    pollValue(getAppNamespaces, (state) => {
      namespacesError = state.error;
      namespacesLoaded = state.loaded;
      if (state.value && state.value.join('\n') !== namespaces.join('\n')) {
        namespaces = state.value;
      }
    }),
  );

  const apps = pollLists('PreviewApp', () => namespaces);
  const grouped = $derived(groupApps(apps.current));
</script>

<Heading size="large" class="mb-4">Apps</Heading>

{#if namespacesError}
  <Alert color={namespacesLoaded ? 'warning' : 'danger'} title="Can't look for apps, retrying">
    {namespacesError.message}
  </Alert>
{/if}
{#if !namespacesLoaded}
  {#if !namespacesError}
    <LoadingSpinner />
  {/if}
{:else if !grouped.loaded}
  <LoadingSpinner />
{:else if grouped.groups.length === 0}
  <Text color="muted">No apps</Text>
{:else}
  <div class="flex flex-col gap-6">
    {#each grouped.groups as group (group.namespace)}
      <section>
        <Heading tag="h2" size="small" class="mb-2">{group.namespace}</Heading>
        {#if group.error}
          <Alert color="warning" title="Can't refresh, retrying">{group.error.message}</Alert>
        {/if}
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {#each group.apps as app (app.metadata?.name)}
            {@const name = app.metadata?.name ?? ''}
            <a href={resolve('/(app)/apps/[namespace]/[app]', { namespace: group.namespace, app: name })}>
              <Card>
                <CardHeader>
                  <div class="flex items-center justify-between gap-2">
                    <CardTitle>{name}</CardTitle>
                    <ReadyBadge object={app} />
                  </div>
                </CardHeader>
              </Card>
            </a>
          {/each}
        </div>
      </section>
    {/each}
  </div>
{/if}
