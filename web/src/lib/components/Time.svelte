<script lang="ts">
  import { DateTime } from 'luxon';

  const { timestamp }: { timestamp: string | undefined } = $props();

  const time = $derived(timestamp ? DateTime.fromISO(timestamp) : undefined);
</script>

{#if time}
  <time datetime={timestamp} title={time.toLocaleString(DateTime.DATETIME_MED)}>{time.toRelative()}</time>
{/if}
