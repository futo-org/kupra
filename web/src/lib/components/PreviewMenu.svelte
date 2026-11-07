<script lang="ts">
  import { ContextMenuButton } from '@immich/ui';
  import { menuItems, previewActions } from '#lib/actions.svelte.ts';
  import type { Preview } from '#lib/kube/types.ts';

  const { preview }: { preview: Preview } = $props();

  const { Edit, Reset, Snapshot, Delete } = previewActions(() => preview);
</script>

<ContextMenuButton aria-label="Actions" items={menuItems([Edit, Reset, Snapshot], [Delete])} />
