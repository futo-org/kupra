<script lang="ts">
  import { Badge, type Color } from '@immich/ui';
  import type { PreviewSnapshot } from '#lib/kube/types.ts';

  const { snapshot }: { snapshot: PreviewSnapshot } = $props();

  const colors: Record<string, Color> = { Succeeded: 'success', InProgress: 'info', Failed: 'danger' };
  const phase = $derived(snapshot.status?.phase);
</script>

<Badge size="small" color={colors[phase ?? ''] ?? 'secondary'}>{phase ?? 'Unknown'}</Badge>
