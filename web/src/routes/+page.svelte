<script lang="ts">
  import { Alert, Card, CardHeader, CardTitle, Heading, LoadingSpinner, Text } from '@immich/ui';
  import { resolve } from '$app/paths';
  import { getAppNamespaces } from '#lib/api/index.ts';
  import ReadyBadge from '#lib/components/ReadyBadge.svelte';
  import { groupApps } from '#lib/kube/apps.ts';
  import { watchLists } from '#lib/kube/watch.svelte.ts';

  let namespaces = $state<string[]>([]);
  let namespacesError = $state<Error>();
  let namespacesLoaded = $state(false);

  $effect(() => {
    const controller = new AbortController();
    const load = async () => {
      try {
        namespaces = await getAppNamespaces(controller.signal);
        namespacesLoaded = true;
      } catch (error) {
        if (!controller.signal.aborted) {
          namespacesError = error instanceof Error ? error : new Error(String(error));
        }
      }
    };
    void load();
    return () => controller.abort();
  });

  const apps = watchLists('PreviewApp', () => namespaces);
  const grouped = $derived(groupApps(apps.current));
</script>

<Heading size="large" class="mb-4">Apps</Heading>

{#if namespacesError}
  <Alert color="danger" title="Can't load apps">{namespacesError.message}</Alert>
{:else if !namespacesLoaded || !grouped.loaded}
  <LoadingSpinner />
{:else if grouped.groups.length === 0}
  <Text color="muted">No apps</Text>
{:else}
  <div class="flex flex-col gap-6">
    {#each grouped.groups as group (group.namespace)}
      <section>
        <Heading tag="h2" size="small" class="mb-2">{group.namespace}</Heading>
        {#if group.error}
          <Alert color="warning" title="Live updates stopped, retrying">{group.error.message}</Alert>
        {/if}
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {#each group.apps as app (app.metadata?.name)}
            {@const name = app.metadata?.name ?? ''}
            <a href={resolve('/apps/[namespace]/[app]', { namespace: group.namespace, app: name })}>
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
