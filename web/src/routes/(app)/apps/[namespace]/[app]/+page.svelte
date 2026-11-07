<script lang="ts">
  import {
    Alert,
    Breadcrumbs,
    Heading,
    Table,
    TableBody,
    TableCell,
    TableHeader,
    TableHeading,
    TableRow,
    Text,
  } from '@immich/ui';
  import { resolve } from '$app/paths';
  import { clock } from '#lib/clock.svelte.ts';
  import Links from '#lib/components/Links.svelte';
  import LoadState from '#lib/components/LoadState.svelte';
  import PhaseBadge from '#lib/components/PhaseBadge.svelte';
  import ReadyBadge from '#lib/components/ReadyBadge.svelte';
  import { age } from '#lib/format.ts';
  import { nameSelector } from '#lib/kube/paths.ts';
  import { ownerLabel, owners, previewsOf } from '#lib/kube/preview.ts';
  import { watchList } from '#lib/kube/watch.svelte.ts';
  import type { PageProps } from './$types';

  const { params }: PageProps = $props();

  const apps = watchList(() => ({
    kind: 'PreviewApp',
    namespace: params.namespace,
    fieldSelector: nameSelector(params.app),
  }));
  const previews = watchList(() => ({ kind: 'Preview', namespace: params.namespace }));
  const app = $derived(apps.current.items[0]);
  const appPreviews = $derived(previewsOf(previews.current.items, params.app));
</script>

<Breadcrumbs
  class="mb-2"
  items={[{ title: 'Apps', href: resolve('/(app)') }, { title: params.namespace }, { title: params.app }]}
/>

<div class="mb-4 flex items-center gap-3">
  <Heading size="large">{params.app}</Heading>
  {#if app}
    <ReadyBadge object={app} />
  {/if}
</div>

<LoadState loaded={apps.current.loaded} error={apps.current.error} what={`apps in ${params.namespace}`}>
  {#if !app}
    <Alert color="warning" title={`PreviewApp ${params.app} not found`} />
  {:else}
    <Heading tag="h2" size="small" class="mb-2">Previews</Heading>
    <LoadState loaded={previews.current.loaded} error={previews.current.error} what={`previews in ${params.namespace}`}>
      {#if appPreviews.length === 0}
        <Text color="muted">No previews</Text>
      {:else}
        <Table>
          <TableHeader>
            <TableHeading>Name</TableHeading>
            <TableHeading>Phase</TableHeading>
            <TableHeading>Ready</TableHeading>
            <TableHeading>Links</TableHeading>
            <TableHeading>Owners</TableHeading>
            <TableHeading>Restored from</TableHeading>
            <TableHeading>Age</TableHeading>
          </TableHeader>
          <TableBody>
            {#each appPreviews as preview (preview.metadata?.uid)}
              {@const name = preview.metadata?.name ?? ''}
              <TableRow>
                <TableCell>
                  <a
                    class="underline"
                    href={resolve('/(app)/apps/[namespace]/[app]/[preview]', { ...params, preview: name })}
                  >
                    {name}
                  </a>
                </TableCell>
                <TableCell><PhaseBadge phase={preview.status?.phase} /></TableCell>
                <TableCell><ReadyBadge object={preview} /></TableCell>
                <TableCell><Links links={preview.status?.links} /></TableCell>
                <TableCell>{owners(preview).map(ownerLabel).join(', ')}</TableCell>
                <TableCell>{preview.status?.restoredFrom?.name ?? ''}</TableCell>
                <TableCell>{age(preview.metadata?.creationTimestamp, clock.now)}</TableCell>
              </TableRow>
            {/each}
          </TableBody>
        </Table>
      {/if}
    </LoadState>
  {/if}
</LoadState>
