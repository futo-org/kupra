<script lang="ts">
  import { Badge } from '@immich/ui';
  import { findCondition, readyColor, readyLabel } from '#lib/kube/conditions.ts';
  import type { Condition } from '#lib/kube/meta.ts';

  const { object }: { object: { status?: { conditions?: Condition[] } } } = $props();
</script>

<Badge size="small" color={readyColor(object)} title={findCondition(object, 'Ready')?.message}>
  {readyLabel(object)}
</Badge>
