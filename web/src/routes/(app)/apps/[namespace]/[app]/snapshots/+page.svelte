<script lang="ts">
  import { Link, Stack, Table, TableBody, TableCell, TableHeader, TableHeading, TableRow, Text } from '@immich/ui';
  import AppPage from '#lib/components/AppPage.svelte';
  import SnapshotMenu from '#lib/components/SnapshotMenu.svelte';
  import SnapshotPhaseBadge from '#lib/components/SnapshotPhaseBadge.svelte';
  import Time from '#lib/components/Time.svelte';
  import Watched from '#lib/components/Watched.svelte';
  import { findCondition } from '#lib/kube/conditions.ts';
  import { collectionsIn, findApp, findPreview, heldBy, isDeleted, snapshotsOf } from '#lib/model.ts';
  import { Route } from '#lib/routes.ts';
  import type { PageProps } from './$types';

  const { params }: PageProps = $props();

  const app = $derived(findApp(params.namespace, params.app));
  const snapshots = $derived(snapshotsOf(params.namespace, params.app));

  const columns = {
    name: 'flex-[4] break-words',
    source: 'hidden flex-[2] break-words md:block',
    phase: 'flex-[3] break-words',
    age: 'hidden flex-[2] md:block',
    menu: 'flex w-12 flex-none justify-end',
  };
</script>

<Watched collections={collectionsIn(params.namespace)}>
  {#if app}
    <AppPage {app} tab="snapshots">
      {#if snapshots.length === 0}
        <Text color="muted" class="py-8 text-center">No snapshots</Text>
      {:else}
        <Table striped spacing="small" size="small">
          <TableHeader>
            <TableHeading class="{columns.name} px-1">Name</TableHeading>
            <TableHeading class="{columns.source} px-1">Source preview</TableHeading>
            <TableHeading class="{columns.phase} px-1">Phase</TableHeading>
            <TableHeading class="{columns.age} px-1">Age</TableHeading>
            <TableHeading class="{columns.menu} px-1"></TableHeading>
          </TableHeader>
          <TableBody>
            {#each snapshots as snapshot (snapshot.metadata?.name)}
              {@const sourceName = snapshot.spec.source.previewRef.name}
              {@const sourcePreview = findPreview(params.namespace, sourceName)}
              {@const ready = findCondition(snapshot, 'Ready')}
              {@const holder = heldBy(snapshot)}
              <TableRow>
                <TableCell class={columns.name}>{snapshot.metadata?.name}</TableCell>
                <TableCell class={columns.source}>
                  {#if sourcePreview}
                    <Link href={Route.preview(params.namespace, sourcePreview.spec.appRef.name, sourceName)}>
                      {sourceName}
                    </Link>
                  {:else}
                    <Text size="small" color="muted"
                      >{sourceName}{isDeleted(params.namespace, sourceName) ? ' (deleted)' : ''}</Text
                    >
                  {/if}
                </TableCell>
                <TableCell class={columns.phase}>
                  <Stack gap={1} class="items-center">
                    <SnapshotPhaseBadge {snapshot} />
                    {#if holder}
                      <Text size="tiny" color="muted">Waiting: {holder}</Text>
                    {:else if snapshot.status?.phase === 'Failed' && ready}
                      <Text size="tiny" color="danger" class="line-clamp-2">{ready.message}</Text>
                    {/if}
                  </Stack>
                </TableCell>
                <TableCell class={columns.age}>
                  <Text size="small"><Time timestamp={snapshot.metadata?.creationTimestamp} /></Text>
                </TableCell>
                <TableCell class={columns.menu}>
                  <SnapshotMenu {snapshot} />
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
