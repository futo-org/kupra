<script lang="ts">
  import { AppShell, AppShellHeader, Button, Container, Heading, Text, ThemeSwitcher } from '@immich/ui';
  import { resolve } from '$app/paths';
  import { loginPath } from '#lib/api/client.ts';
  import { logout } from '#lib/api/index.ts';
  import type { LayoutProps } from './$types';

  const { data, children }: LayoutProps = $props();

  let signedOut = $state(false);

  const signOut = async () => {
    await logout();
    signedOut = true;
  };
</script>

<AppShell>
  <AppShellHeader>
    <div class="flex w-full items-center gap-4 px-4">
      <a href={resolve('/(app)')} class="font-semibold">kupra</a>
      <div class="ml-auto flex items-center gap-2">
        {#if !signedOut}
          <Text size="small" color="muted">{data.session.username}</Text>
          <Button size="small" variant="ghost" color="secondary" onclick={signOut}>Sign out</Button>
        {/if}
        <ThemeSwitcher />
      </div>
    </div>
  </AppShellHeader>
  <Container size="giant" class="p-4">
    {#if signedOut}
      <Heading size="large">Signed out</Heading>
      <Button class="mt-4" href={loginPath(resolve('/(app)'))}>Sign in</Button>
    {:else}
      {@render children()}
    {/if}
  </Container>
</AppShell>
