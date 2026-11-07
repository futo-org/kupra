<script lang="ts">
  import {
    AppShell,
    AppShellHeader,
    AppShellSidebar,
    ContextMenuButton,
    IconButton,
    MenuItemType,
    NavbarGroup,
    NavbarItem,
    Text,
    ThemeSwitcher,
    toastManager,
    type ActionItem,
  } from '@immich/ui';
  import {
    mdiAccountCircleOutline,
    mdiCheck,
    mdiHomeOutline,
    mdiLogout,
    mdiMenu,
    mdiViewDashboardOutline,
  } from '@mdi/js';
  import { MediaQuery } from 'svelte/reactivity';
  import { afterNavigate } from '$app/navigation';
  import { page } from '$app/state';
  import { user, type Role } from '#lib/mock/data.ts';
  import { appGroups } from '#lib/mock/model.ts';
  import { Route } from '#lib/mock/routes.ts';
  import { access } from '#lib/mock/store.svelte.ts';
  import type { LayoutProps } from './$types';

  const { children }: LayoutProps = $props();

  const wide = new MediaQuery('min-width: 768px');
  let sidebarOpen = $derived(wide.current);
  afterNavigate(() => {
    sidebarOpen = wide.current;
  });

  const isUnder = (href: string) => page.url.pathname === href || page.url.pathname.startsWith(`${href}/`);

  // The mockup lets you switch roles, to see what each one may do.
  const roleItem = (role: Role): ActionItem => ({
    title: `View as ${role}`,
    icon: access.roles['immich-previews'] === role ? mdiCheck : undefined,
    onAction: () => {
      access.roles['immich-previews'] = role;
    },
  });
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
        <Text size="small" color="muted" class="hidden sm:block">{user.username}</Text>
        <ThemeSwitcher />
        <ContextMenuButton
          icon={mdiAccountCircleOutline}
          aria-label="Account"
          items={[
            roleItem('viewer'),
            roleItem('creator'),
            roleItem('admin'),
            MenuItemType.Divider,
            {
              title: 'Sign out',
              icon: mdiLogout,
              onAction: () => toastManager.info('Signing out is off in the mockup'),
            },
          ]}
        />
      </div>
    </div>
  </AppShellHeader>
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
          <NavbarItem
            title={app.metadata?.name ?? ''}
            href={Route.app(group.namespace, app.metadata?.name ?? '')}
            icon={mdiViewDashboardOutline}
            isActive={() => isUnder(Route.app(group.namespace, app.metadata?.name ?? ''))}
          />
        {/each}
      {/each}
    </div>
  </AppShellSidebar>
  {@render children()}
</AppShell>
