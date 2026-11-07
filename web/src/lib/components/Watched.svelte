<script lang="ts">
  import { Alert, LoadingSpinner } from '@immich/ui';
  import type { Snippet } from 'svelte';
  import { store } from '#lib/watch.svelte.ts';

  type Props = {
    /** collections are what the content shows, which it waits for. */
    collections: string[];
    children: Snippet;
  };

  const { collections, children }: Props = $props();

  store.want(() => collections);

  const states = $derived(collections.map((name) => ({ name, collection: store.collection(name) })));
  const errors = $derived(states.flatMap(({ collection }) => (collection?.error ? [collection.error] : [])));
  const synced = $derived(states.every(({ collection }) => collection?.synced || collection?.error));
</script>

{#each errors as error, i (i)}
  <Alert color={error.code === 403 ? 'warning' : 'danger'} title={error.message ?? error.reason ?? 'Error'} />
{/each}
{#if synced}
  {@render children()}
{:else}
  <LoadingSpinner />
{/if}
