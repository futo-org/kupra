<script lang="ts">
  import { ActionButton, Breadcrumbs, Container, type ActionItem, type BreadcrumbItem } from '@immich/ui';
  import { mdiSlashForward } from '@mdi/js';
  import type { Snippet } from 'svelte';

  type Props = {
    breadcrumbs: BreadcrumbItem[];
    /** actions are shown as buttons at the end of the breadcrumbs. */
    actions?: ActionItem[];
    children: Snippet;
  };

  const { breadcrumbs, actions = [], children }: Props = $props();
</script>

<div class="flex min-h-16 w-full flex-wrap items-center justify-between gap-2 border-b px-4 py-2">
  <Breadcrumbs items={breadcrumbs} separator={mdiSlashForward} />
  <div class="flex items-center gap-2">
    {#each actions as action (action.title)}
      <ActionButton {action} type="button" shape="round" variant={action.color === 'primary' ? 'filled' : 'ghost'} />
    {/each}
  </div>
</div>
<Container size="giant" class="flex flex-col gap-6 p-4 pb-16">
  {@render children()}
</Container>
