<script lang="ts">
  import { TooltipProvider } from '@immich/ui';
  import '../app.css';
  import type { LayoutProps } from './$types';

  const { children }: LayoutProps = $props();
</script>

<TooltipProvider>
  {@render children()}
</TooltipProvider>
