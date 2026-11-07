<script lang="ts">
  import { Badge } from '@immich/ui';
  import { phaseColor, type Phase } from '#lib/kube/preview.ts';

  const { phase }: { phase: Phase | undefined } = $props();
</script>

<Badge size="small" color={phaseColor(phase)}>{phase ?? 'Unknown'}</Badge>
