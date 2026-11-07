<script lang="ts">
  import {
    Alert,
    Badge,
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
  import { conditionColor } from '#lib/kube/conditions.ts';
  import { nameSelector } from '#lib/kube/paths.ts';
  import { ownerLabel, owners, parameterRows } from '#lib/kube/preview.ts';
  import { watchList } from '#lib/kube/watch.svelte.ts';
  import type { PageProps } from './$types';

  const { params }: PageProps = $props();

  const previews = watchList(() => ({
    kind: 'Preview',
    namespace: params.namespace,
    fieldSelector: nameSelector(params.preview),
  }));
  // The app only adds parameter titles and defaults, so the page works without it.
  const apps = watchList(() => ({
    kind: 'PreviewApp',
    namespace: params.namespace,
    fieldSelector: nameSelector(params.app),
  }));
  const preview = $derived(previews.current.items[0]);
  const parameters = $derived(preview ? parameterRows(preview, apps.current.items[0]) : []);
</script>

<Breadcrumbs
  class="mb-2"
  items={[
    { title: 'Apps', href: resolve('/(app)') },
    { title: params.namespace },
    { title: params.app, href: resolve('/(app)/apps/[namespace]/[app]', params) },
    { title: params.preview },
  ]}
/>

<div class="mb-4 flex items-center gap-3">
  <Heading size="large">{params.preview}</Heading>
  {#if preview}
    <PhaseBadge phase={preview.status?.phase} />
    <ReadyBadge object={preview} />
  {/if}
</div>

<LoadState loaded={previews.current.loaded} error={previews.current.error} what={`previews in ${params.namespace}`}>
  {#if !preview}
    <Alert color="warning" title={`Preview ${params.preview} not found`} />
  {:else}
    <div class="flex flex-col gap-6">
      <dl class="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-1">
        <dt class="font-medium">Links</dt>
        <dd><Links links={preview.status?.links} /></dd>
        <dt class="font-medium">Owners</dt>
        <dd>{owners(preview).map(ownerLabel).join(', ')}</dd>
        <dt class="font-medium">Restored from</dt>
        <dd>{preview.status?.restoredFrom?.name ?? ''}</dd>
        <dt class="font-medium">Namespace</dt>
        <dd>{preview.status?.namespace ?? ''}</dd>
        <dt class="font-medium">Age</dt>
        <dd>{age(preview.metadata?.creationTimestamp, clock.now)}</dd>
      </dl>

      <section>
        <Heading tag="h2" size="small" class="mb-2">Conditions</Heading>
        {#if preview.status?.conditions?.length}
          <Table>
            <TableHeader>
              <TableHeading>Type</TableHeading>
              <TableHeading>Status</TableHeading>
              <TableHeading>Reason</TableHeading>
              <TableHeading>Message</TableHeading>
              <TableHeading>Last transition</TableHeading>
            </TableHeader>
            <TableBody>
              {#each preview.status.conditions as condition (condition.type)}
                <TableRow>
                  <TableCell>{condition.type}</TableCell>
                  <TableCell><Badge size="small" color={conditionColor(condition)}>{condition.status}</Badge></TableCell
                  >
                  <TableCell>{condition.reason}</TableCell>
                  <TableCell class="whitespace-pre-wrap">{condition.message}</TableCell>
                  <TableCell>{age(condition.lastTransitionTime, clock.now)}</TableCell>
                </TableRow>
              {/each}
            </TableBody>
          </Table>
        {:else}
          <Text color="muted">No conditions</Text>
        {/if}
      </section>

      <section>
        <Heading tag="h2" size="small" class="mb-2">Parameters</Heading>
        {#if parameters.length > 0}
          <Table>
            <TableHeader>
              <TableHeading>Name</TableHeading>
              <TableHeading>Value</TableHeading>
            </TableHeader>
            <TableBody>
              {#each parameters as parameter (parameter.name)}
                <TableRow>
                  <TableCell title={parameter.name}>{parameter.title ?? parameter.name}</TableCell>
                  <TableCell>
                    <code>{parameter.value}</code>
                    {#if parameter.isDefault}
                      <Text inline size="small" color="muted">(default)</Text>
                    {/if}
                  </TableCell>
                </TableRow>
              {/each}
            </TableBody>
          </Table>
        {:else}
          <Text color="muted">No parameters</Text>
        {/if}
      </section>

      <section>
        <Heading tag="h2" size="small" class="mb-2">Inventory</Heading>
        {#if preview.status?.inventory?.length}
          <Table>
            <TableHeader>
              <TableHeading>Kind</TableHeading>
              <TableHeading>Name</TableHeading>
              <TableHeading>State</TableHeading>
            </TableHeader>
            <TableBody>
              {#each preview.status.inventory as entry (`${entry.apiGroup ?? ''}/${entry.kind}/${entry.name}`)}
                <TableRow>
                  <TableCell>{entry.apiGroup ? `${entry.kind}.${entry.apiGroup}` : entry.kind}</TableCell>
                  <TableCell>{entry.name}</TableCell>
                  <TableCell>
                    {#if entry.retained}
                      Retained
                    {:else if entry.state}
                      State
                    {/if}
                  </TableCell>
                </TableRow>
              {/each}
            </TableBody>
          </Table>
        {:else}
          <Text color="muted">No resources</Text>
        {/if}
      </section>
    </div>
  {/if}
</LoadState>
