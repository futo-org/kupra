<script lang="ts">
  import { Alert, Button, HStack, Heading } from '@immich/ui';
  import type { Snippet } from 'svelte';
  import { findCondition } from '#lib/kube/conditions.ts';
  import type { PreviewApp } from '#lib/kube/kupra.gen.ts';
  import { getAppActions } from '../actions.ts';
  import { Route } from '../routes.ts';
  import PageLayout from './PageLayout.svelte';

  type Props = { app: PreviewApp; tab: 'previews' | 'snapshots'; children: Snippet };

  const { app, tab, children }: Props = $props();

  const namespace = $derived(app.metadata?.namespace ?? '');
  const name = $derived(app.metadata?.name ?? '');
  const ready = $derived(findCondition(app, 'Ready'));
  const { Create } = $derived(getAppActions(app));

  const tabs = $derived([
    { id: 'previews', title: 'Previews', href: Route.app(namespace, name) },
    { id: 'snapshots', title: 'Snapshots', href: Route.snapshots(namespace, name) },
  ]);
</script>

<PageLayout breadcrumbs={[{ title: namespace }, { title: name }]} actions={[Create]}>
  <div class="flex flex-col gap-4">
    <Heading tag="h1" size="large">{name}</Heading>
    {#if ready && ready.status !== 'True'}
      <Alert color="danger" title="App isn't ready">{ready.message}</Alert>
    {/if}
    <HStack gap={1}>
      {#each tabs as { id, title, href } (id)}
        <Button
          {href}
          size="small"
          shape="round"
          color={id === tab ? 'primary' : 'secondary'}
          variant={id === tab ? 'filled' : 'ghost'}
          aria-current={id === tab ? 'page' : undefined}
        >
          {title}
        </Button>
      {/each}
    </HStack>
  </div>
  {@render children()}
</PageLayout>
