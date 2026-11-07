<script lang="ts">
  import { Button, type Size } from '@immich/ui';
  import { mdiOpenInNew } from '@mdi/js';

  type Props = { links: { name: string; url: string }[] | undefined; size?: Size };

  const { links = [], size = 'small' }: Props = $props();
</script>

{#each links as link, i (link.name)}
  <Button
    href={link.url}
    target="_blank"
    rel="noreferrer"
    {size}
    shape="round"
    variant={i === 0 ? 'filled' : 'outline'}
    trailingIcon={mdiOpenInNew}
  >
    {link.name}
  </Button>
{/each}
