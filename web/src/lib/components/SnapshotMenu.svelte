<script lang="ts">
  import { ContextMenuButton } from '@immich/ui';
  import { menuItems, snapshotActions } from '#lib/actions.svelte.ts';
  import type { PreviewSnapshot } from '#lib/kube/types.ts';

  const { snapshot }: { snapshot: PreviewSnapshot } = $props();

  const { Delete } = snapshotActions(() => snapshot);
</script>

<ContextMenuButton aria-label="Actions" items={menuItems([Delete])} />
