<script lang="ts">
  import { Link, Stack, Table, TableBody, TableCell, TableHeader, TableHeading, TableRow, Text } from '@immich/ui';
  import AppPage from '#lib/components/AppPage.svelte';
  import LinkButtons from '#lib/components/LinkButtons.svelte';
  import PhaseBadge from '#lib/components/PhaseBadge.svelte';
  import PreviewMenu from '#lib/components/PreviewMenu.svelte';
  import Time from '#lib/components/Time.svelte';
  import Watched from '#lib/components/Watched.svelte';
  import { findCondition } from '#lib/kube/conditions.ts';
  import { ownerLabel, owners } from '#lib/kube/preview.ts';
  import { collectionsIn, findApp, isFailing, previewsOf, source } from '#lib/model.ts';
  import { Route } from '#lib/routes.ts';
  import type { PageProps } from './$types';

  const { params }: PageProps = $props();

  const app = $derived(findApp(params.namespace, params.app));
  const previews = $derived(previewsOf(params.namespace, params.app));

  const columns = {
    name: 'flex-[10] break-words',
    status: 'flex-[11] break-words',
    links: 'hidden flex-[12] flex-wrap justify-center gap-1 md:flex',
    source: 'hidden flex-[10] break-words lg:block',
    restored: 'hidden flex-[9] break-words xl:block',
    owners: 'hidden flex-[10] break-words lg:block',
    age: 'hidden flex-[7] break-words md:block',
    menu: 'flex w-12 flex-none justify-end',
  };
</script>

<Watched collections={collectionsIn(params.namespace)}>
  {#if app}
    <AppPage {app} tab="previews">
      {#if previews.length === 0}
        <Text color="muted" class="py-8 text-center">No previews</Text>
      {:else}
        <Table striped spacing="small" size="small">
          <TableHeader>
            <TableHeading class="{columns.name} px-1">Name</TableHeading>
            <TableHeading class="{columns.status} px-1">Status</TableHeading>
            <TableHeading class="{columns.links} px-1">Links</TableHeading>
            <TableHeading class="{columns.source} px-1">Source</TableHeading>
            <TableHeading class="{columns.restored} px-1">Started from</TableHeading>
            <TableHeading class="{columns.owners} px-1">Owners</TableHeading>
            <TableHeading class="{columns.age} px-1">Age</TableHeading>
            <TableHeading class="{columns.menu} px-1"></TableHeading>
          </TableHeader>
          <TableBody>
            {#each previews as preview (preview.metadata?.name)}
              {@const name = preview.metadata?.name ?? ''}
              {@const ready = findCondition(preview, 'Ready')}
              {@const from = source(preview)}
              <TableRow>
                <TableCell class={columns.name}>
                  <Link href={Route.preview(params.namespace, params.app, name)}>{name}</Link>
                </TableCell>
                <TableCell class={columns.status}>
                  <Stack gap={1} class="items-center">
                    <PhaseBadge phase={preview.status?.phase} />
                    {#if ready && ready.status !== 'True'}
                      <Text size="tiny" color={isFailing(preview) ? 'danger' : 'muted'} class="line-clamp-2">
                        {ready.message}
                      </Text>
                    {/if}
                  </Stack>
                </TableCell>
                <TableCell class={columns.links}>
                  <LinkButtons links={preview.status?.links} size="tiny" />
                </TableCell>
                <TableCell class={columns.source}>
                  {#if from}
                    <Link href={from.url} target="_blank" rel="noreferrer">{from.title}</Link>
                  {:else}
                    <Text size="small" color="muted">Unknown</Text>
                  {/if}
                </TableCell>
                <TableCell class={columns.restored}>
                  {#if preview.status?.restoredFrom}
                    <Text size="small">{preview.status.restoredFrom.name}</Text>
                  {:else}
                    <Text size="small" color="muted">Empty</Text>
                  {/if}
                </TableCell>
                <TableCell class={columns.owners}>
                  <Text size="small">{owners(preview).map(ownerLabel).join(', ')}</Text>
                </TableCell>
                <TableCell class={columns.age}>
                  <Text size="small"><Time timestamp={preview.metadata?.creationTimestamp} /></Text>
                </TableCell>
                <TableCell class={columns.menu}>
                  <PreviewMenu {preview} />
                </TableCell>
              </TableRow>
            {/each}
          </TableBody>
        </Table>
      {/if}
    </AppPage>
  {:else}
    <Text color="muted" class="p-4">App {params.app} not found</Text>
  {/if}
</Watched>
