<script lang="ts">
  import { Link } from '@immich/ui';

  const { links = [] }: { links?: { name: string; url: string }[] } = $props();
</script>

<span class="flex flex-wrap gap-x-3">
  {#each links as link (link.name)}
    <Link href={link.url}>{link.name}</Link>
  {/each}
</span>
