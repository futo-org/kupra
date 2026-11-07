<script lang="ts">
  import { Alert, LoadingSpinner } from '@immich/ui';
  import type { Snippet } from 'svelte';
  import { isForbidden } from '#lib/api/client.ts';

  type Props = {
    loaded: boolean;
    error?: Error;
    /** what names what's loaded, as in "You can't see previews in foo". */
    what: string;
    children: Snippet;
  };

  const { loaded, error, what, children }: Props = $props();
</script>

{#if error && !loaded}
  {#if isForbidden(error)}
    <Alert color="warning" title={`You can't see ${what}`} />
  {:else}
    <Alert color="danger" title={`Can't load ${what}`}>{error.message}</Alert>
  {/if}
{:else if !loaded}
  <LoadingSpinner />
{:else}
  {#if error}
    <Alert color="warning" title="Live updates stopped, retrying">{error.message}</Alert>
  {/if}
  {@render children()}
{/if}
