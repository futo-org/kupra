<script lang="ts">
  import { TooltipProvider } from '@immich/ui';
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import { queryClient } from '#lib/queries.ts';
  import '../app.css';
  import type { LayoutProps } from './$types';

  const { children }: LayoutProps = $props();
</script>

<QueryClientProvider client={queryClient}>
  <TooltipProvider>
    {@render children()}
  </TooltipProvider>
</QueryClientProvider>
