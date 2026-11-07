<script lang="ts">
  import {
    Alert,
    AppShell,
    AppShellHeader,
    AppShellSidebar,
    Button,
    Container,
    ContextMenuButton,
    Heading,
    IconButton,
    NavbarGroup,
    NavbarItem,
    Text,
    ThemeSwitcher,
  } from '@immich/ui';
  import { mdiAccountCircleOutline, mdiHomeOutline, mdiLogout, mdiMenu, mdiViewDashboardOutline } from '@mdi/js';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { MediaQuery } from 'svelte/reactivity';
  import { afterNavigate } from '$app/navigation';
  import { page } from '$app/state';
  import { request, signIn } from '#lib/api.ts';
  import { appGroups, namespaces } from '#lib/model.ts';
  import { sessionQuery } from '#lib/queries.ts';
  import { Route } from '#lib/routes.ts';
  import { store } from '#lib/watch.svelte.ts';
  import type { LayoutProps } from './$types';

  const { children }: LayoutProps = $props();

  const session = createQuery(() => sessionQuery);
  let signedOut = $state(false);
  const signOut = createMutation(() => ({
    mutationFn: () => request('/auth/logout', { method: 'POST' }),
    onSuccess: () => (signedOut = true),
  }));

  // The sidebar lists the apps in every namespace that has them.
  store.want(() => ['app-namespaces', ...namespaces().map((namespace) => `previewapps/${namespace}`)]);

  // The page streams everything its components ask for over one connection, reconnecting when that changes.
  // Components ask for collections again whenever what they show changes, so only a different set reconnects.
  const wanted = $derived(store.wanted().join('&'));
  $effect(() => {
    if (!signedOut && wanted) {
      return store.connect(wanted.split('&'));
    }
  });

  const wide = new MediaQuery('min-width: 768px');
  let sidebarOpen = $derived(wide.current);
  afterNavigate(() => {
    sidebarOpen = wide.current;
  });

  const isUnder = (href: string) => page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);
</script>

<AppShell>
  <AppShellHeader>
    <div class="flex w-full items-center gap-2 px-2">
      <IconButton
        icon={mdiMenu}
        shape="round"
        variant="ghost"
        color="secondary"
        aria-label="Menu"
        class="md:hidden"
        onclick={() => (sidebarOpen = !sidebarOpen)}
      />
      <a href={Route.home()} class="px-2 text-xl font-semibold text-primary">kupra</a>
      <div class="ms-auto flex items-center gap-1">
        {#if session.data && !signedOut}
          <Text size="small" color="muted" class="hidden sm:block">{session.data.username}</Text>
        {/if}
        <ThemeSwitcher />
        {#if !signedOut}
          <ContextMenuButton
            icon={mdiAccountCircleOutline}
            aria-label="Account"
            items={[{ title: 'Sign out', icon: mdiLogout, onAction: () => signOut.mutate() }]}
          />
        {/if}
      </div>
    </div>
  </AppShellHeader>
  {#if signedOut}
    <Container size="small" class="flex flex-col items-start gap-4 p-8">
      <Heading size="large">Signed out</Heading>
      <Button shape="round" onclick={signIn}>Sign in</Button>
    </Container>
  {:else}
    <AppShellSidebar bind:open={sidebarOpen}>
      <div class="flex flex-col pt-4 pe-4">
        <NavbarItem
          title="My previews"
          href={Route.home()}
          icon={mdiHomeOutline}
          isActive={() => page.url.pathname === Route.home()}
        />
        {#each appGroups() as group (group.namespace)}
          <NavbarGroup title={group.namespace} size="tiny" variant="compact" />
          {#each group.apps as app (app.metadata?.name)}
            {@const href = Route.app(group.namespace, app.metadata?.name ?? '')}
            <NavbarItem
              title={app.metadata?.name ?? ''}
              {href}
              icon={mdiViewDashboardOutline}
              isActive={() => isUnder(href)}
            />
          {/each}
        {/each}
      </div>
    </AppShellSidebar>
    <div>
      {#if store.disconnected}
        <Alert color="warning" size="small" shape="rectangle" title="Live updates stopped, reconnecting" />
      {/if}
      {#if session.error}
        <Alert color="danger" size="small" shape="rectangle" title="Can't load your session">
          {session.error.message}
        </Alert>
      {/if}
      {@render children()}
    </div>
  {/if}
</AppShell>
