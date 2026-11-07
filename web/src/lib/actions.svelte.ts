// The actions on apps, previews and snapshots, offered only when the API server says the user may take them.
// Each function creates queries and mutations, so it's called while a component initialises.

import {
  ConfirmModal,
  MenuItemType,
  isEnabled,
  modalManager,
  toastManager,
  type ActionItem,
  type MenuItems,
} from '@immich/ui';
import { mdiCameraOutline, mdiPencilOutline, mdiPlus, mdiRestore, mdiTrashCanOutline } from '@mdi/js';
import { createMutation, createQuery } from '@tanstack/svelte-query';
import type { Component } from 'svelte';
import { goto } from '$app/navigation';
import PreviewFormModalComponent from './components/PreviewFormModal.svelte';
import SnapshotModalComponent from './components/SnapshotModal.svelte';
import { accessRevision } from './kube/preview.ts';
import type { Preview, PreviewApp, PreviewSnapshot, ResourceAttributes } from './kube/types.ts';
import { findApp, managedBy, restoreLabel } from './model.ts';
import {
  accessQuery,
  createPreview,
  createSnapshot,
  deletePreview,
  deleteSnapshot,
  resetPreview,
  updatePreview,
} from './queries.ts';
import { Route } from './routes.ts';

// The modals' types, which type-aware linting can't read from .svelte files.
const PreviewFormModal = PreviewFormModalComponent as Component<{
  app: PreviewApp;
  preview?: Preview;
  onClose: (preview?: Preview) => void;
}>;
const SnapshotModal = SnapshotModalComponent as Component<{
  preview: Preview;
  onClose: (snapshot?: PreviewSnapshot) => void;
}>;

const confirm = (title: string, prompt: string, confirmText: string) =>
  modalManager.show(ConfirmModal, { title, prompt, confirmText, icon: false });

/** canI asks whether the user may take an action, and is false until the API server answers. It asks again when revision changes. */
const canI = (attributes: () => ResourceAttributes, revision: () => string = () => '') => {
  const query = createQuery(() => accessQuery(attributes(), revision()));
  return () => query.data === true;
};

/** appActions returns the actions on an app. */
export const appActions = (app: () => PreviewApp) => {
  const namespace = () => app().metadata?.namespace ?? '';
  const canCreate = canI(() => ({ namespace: namespace(), verb: 'create', resource: 'previews' }));
  const create = createMutation(() => ({ mutationFn: createPreview }));

  const Create: ActionItem = {
    title: 'New preview',
    icon: mdiPlus,
    color: 'primary',
    $if: canCreate,
    onAction: async () => {
      const preview = await modalManager.show(PreviewFormModal, { app: app() });
      if (!preview) {
        return;
      }
      const name = preview.metadata?.name ?? '';
      create.mutate(preview, {
        onSuccess: () => {
          toastManager.success(`Created ${name}`);
          void goto(Route.preview(namespace(), app().metadata?.name ?? '', name));
        },
      });
    },
  };

  return { Create };
};

/** previewActions returns the actions on a preview. Automation that manages a preview would undo edits, so those are left out. */
export const previewActions = (preview: () => Preview) => {
  const target = (verb: string) => ({
    namespace: preview().metadata?.namespace,
    verb,
    resource: 'previews',
    name: preview().metadata?.name,
  });
  // Owners can act on a preview once kupra has bound them, which happens after the preview is created or its owners change.
  const revision = () => accessRevision(preview());
  const canPatch = canI(() => target('patch'), revision);
  const canDelete = canI(() => target('delete'), revision);
  // Snapshotting a preview takes kupra's snapshot verb on it, as well as creating snapshots (ADR 6).
  const canSnapshot = canI(() => target('snapshot'), revision);
  const canCreateSnapshots = canI(() => ({
    namespace: preview().metadata?.namespace,
    verb: 'create',
    resource: 'previewsnapshots',
  }));
  const managed = () => managedBy(preview()) !== undefined;
  const name = () => preview().metadata?.name ?? '';

  const update = createMutation(() => ({ mutationFn: updatePreview }));
  const reset = createMutation(() => ({ mutationFn: resetPreview }));
  const remove = createMutation(() => ({ mutationFn: deletePreview }));
  const snapshot = createMutation(() => ({ mutationFn: createSnapshot }));

  const Edit: ActionItem = {
    title: 'Edit',
    icon: mdiPencilOutline,
    // Saving patches the preview, so that's what Edit needs.
    $if: () => canPatch() && !managed(),
    onAction: async () => {
      const app = findApp(preview().metadata?.namespace ?? '', preview().spec.appRef.name);
      const edited = app && (await modalManager.show(PreviewFormModal, { app, preview: preview() }));
      if (edited) {
        update.mutate(edited, { onSuccess: () => toastManager.success(`Saved ${name()}`) });
      }
    },
  };

  const Reset: ActionItem = {
    title: 'Reset',
    icon: mdiRestore,
    $if: canPatch,
    onAction: async () => {
      const from = restoreLabel(preview());
      const prompt = `Its data is replaced with ${from === 'Empty' ? 'empty state' : from}, and it restarts.`;
      if (await confirm(`Reset ${name()}?`, prompt, 'Reset')) {
        reset.mutate(preview(), { onSuccess: () => toastManager.info(`Resetting ${name()}`) });
      }
    },
  };

  const Snapshot: ActionItem = {
    title: 'Take snapshot',
    icon: mdiCameraOutline,
    $if: () => canSnapshot() && canCreateSnapshots(),
    onAction: async () => {
      const taken = await modalManager.show(SnapshotModal, { preview: preview() });
      if (taken) {
        snapshot.mutate(taken, {
          onSuccess: () => toastManager.info(`Taking snapshot ${taken.metadata?.name}`),
        });
      }
    },
  };

  const Delete: ActionItem = {
    title: 'Delete',
    icon: mdiTrashCanOutline,
    color: 'danger',
    $if: () => canDelete() && !managed(),
    onAction: async () => {
      const deleted = preview();
      if (await confirm(`Delete ${name()}?`, 'Its data is deleted with it.', 'Delete')) {
        remove.mutate(deleted, {
          onSuccess: () => {
            toastManager.success(`Deleted ${deleted.metadata?.name}`);
            void goto(Route.app(deleted.metadata?.namespace ?? '', deleted.spec.appRef.name));
          },
        });
      }
    },
  };

  return { Edit, Reset, Snapshot, Delete };
};

/** snapshotActions returns the actions on a snapshot. */
export const snapshotActions = (snapshot: () => PreviewSnapshot) => {
  const canDelete = canI(() => ({
    namespace: snapshot().metadata?.namespace,
    verb: 'delete',
    resource: 'previewsnapshots',
    name: snapshot().metadata?.name,
  }));
  const remove = createMutation(() => ({ mutationFn: deleteSnapshot }));

  const Delete: ActionItem = {
    title: 'Delete',
    icon: mdiTrashCanOutline,
    color: 'danger',
    $if: () => canDelete() && snapshot().status?.phase !== 'Deleting',
    onAction: async () => {
      const deleted = snapshot();
      const name = deleted.metadata?.name ?? '';
      if (await confirm(`Delete ${name}?`, 'Previews can no longer start from it.', 'Delete')) {
        remove.mutate(deleted, { onSuccess: () => toastManager.success(`Deleting ${name}`) });
      }
    },
  };

  return { Delete };
};

/** menuItems returns the enabled actions for a menu, with a divider between groups. */
export const menuItems = (...groups: ActionItem[][]): MenuItems =>
  groups
    .map((group) => group.filter((action) => isEnabled(action)))
    .filter((group) => group.length > 0)
    .flatMap((group, i) => (i === 0 ? group : [MenuItemType.Divider, ...group]));
