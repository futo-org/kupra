<script lang="ts">
  import { Card, CardBody, CardHeader, CardTitle, Link, Stack, Text } from '@immich/ui';
  import { mdiHomeOutline } from '@mdi/js';
  import { createQuery } from '@tanstack/svelte-query';
  import LinkButtons from '#lib/components/LinkButtons.svelte';
  import PageLayout from '#lib/components/PageLayout.svelte';
  import PhaseBadge from '#lib/components/PhaseBadge.svelte';
  import Time from '#lib/components/Time.svelte';
  import Watched from '#lib/components/Watched.svelte';
  import { findCondition } from '#lib/kube/conditions.ts';
  import { isFailing, myPreviews, namespaces, source } from '#lib/model.ts';
  import { sessionQuery } from '#lib/queries.ts';
  import { Route } from '#lib/routes.ts';

  const session = createQuery(() => sessionQuery);
  const previews = $derived(session.data ? myPreviews(session.data) : []);
  const collections = $derived(['app-namespaces', ...namespaces().map((namespace) => `previews/${namespace}`)]);
</script>

<PageLayout breadcrumbs={[{ title: 'My previews', icon: mdiHomeOutline }]}>
  <Watched {collections}>
    {#if previews.length === 0}
      <Stack gap={1} class="py-16 text-center">
        <Text fontWeight="medium">No previews yet</Text>
        <Text size="small" color="muted">Previews you create or own show up here. Pick an app to create one.</Text>
      </Stack>
    {:else}
      <div class="grid items-start gap-4 md:grid-cols-2 2xl:grid-cols-3">
        {#each previews as preview (`${preview.metadata?.namespace}/${preview.metadata?.name}`)}
          {@const namespace = preview.metadata?.namespace ?? ''}
          {@const app = preview.spec.appRef.name}
          {@const ready = findCondition(preview, 'Ready')}
          {@const from = source(preview)}
          <Card>
            <CardHeader>
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <Link href={Route.preview(namespace, app, preview.metadata?.name ?? '')} underline={false}>
                    <CardTitle class="truncate">{preview.metadata?.name}</CardTitle>
                  </Link>
                  <Text size="small" color="muted">
                    {app}
                    {#if from}
                      · <Link href={from.url} target="_blank" rel="noreferrer">{from.title}</Link>
                    {/if}
                    · <Time timestamp={preview.metadata?.creationTimestamp} />
                  </Text>
                </div>
                <PhaseBadge phase={preview.status?.phase} />
              </div>
            </CardHeader>
            <CardBody>
              <Stack gap={3}>
                {#if ready && ready.status !== 'True'}
                  <Text size="small" color={isFailing(preview) ? 'danger' : 'muted'} class="line-clamp-2">
                    {ready.message}
                  </Text>
                {/if}
                {#if preview.status?.links?.length}
                  <div class="flex flex-wrap gap-2">
                    <LinkButtons links={preview.status.links} />
                  </div>
                {/if}
              </Stack>
            </CardBody>
          </Card>
        {/each}
      </div>
    {/if}
  </Watched>
</PageLayout>
