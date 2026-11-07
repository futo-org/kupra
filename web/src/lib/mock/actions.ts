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
import type { Component } from 'svelte';
import { goto } from '$app/navigation';
import type { Preview, PreviewApp, PreviewSnapshot } from '#lib/kube/kupra.gen.ts';
import PreviewFormModalComponent from './components/PreviewFormModal.svelte';
import SnapshotModalComponent from './components/SnapshotModal.svelte';
import { now } from './data.ts';
import { canCreate, canDeleteSnapshots, findApp, permissions, restoreLabel } from './model.ts';
import { Route } from './routes.ts';
import { list, send } from './store.svelte.ts';

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

const resetAnnotation = 'kupra.futo.tech/reset-requested-at';

const confirm = (title: string, prompt: string, confirmText: string) =>
  modalManager.show(ConfirmModal, { title, prompt, confirmText, icon: false });

/** getAppActions returns the actions on an app. */
export const getAppActions = (app: PreviewApp) => {
  const namespace = app.metadata?.namespace ?? '';

  const Create: ActionItem = {
    title: 'New preview',
    icon: mdiPlus,
    color: 'primary',
    $if: () => canCreate(namespace),
    onAction: async () => {
      const preview = await modalManager.show(PreviewFormModal, { app });
      if (!preview) {
        return;
      }
      const name = preview.metadata?.name ?? '';
      send('previews', 'ADDED', {
        ...preview,
        metadata: { ...preview.metadata, creationTimestamp: now() },
        status: { phase: 'Pending' },
      });
      toastManager.success(`Created ${name}`);
      await goto(Route.preview(namespace, app.metadata?.name ?? '', name));
    },
  };

  return { Create };
};

/** getPreviewActions returns the actions on a preview, apart from those the user may not take. */
export const getPreviewActions = (preview: Preview) => {
  const namespace = preview.metadata?.namespace ?? '';
  const name = preview.metadata?.name ?? '';
  const can = permissions(preview);

  const Edit: ActionItem = {
    title: 'Edit',
    icon: mdiPencilOutline,
    $if: () => can.edit,
    onAction: async () => {
      const app = findApp(namespace, preview.spec.appRef.name);
      const edited = app && (await modalManager.show(PreviewFormModal, { app, preview }));
      if (edited) {
        send('previews', 'MODIFIED', { ...preview, spec: edited.spec });
        toastManager.success(`Saved ${name}`);
      }
    },
  };

  const Reset: ActionItem = {
    title: 'Reset',
    icon: mdiRestore,
    $if: () => can.reset,
    onAction: async () => {
      const from = restoreLabel(preview);
      const prompt = `Its data is replaced with ${from === 'Empty' ? 'empty state' : from}, and it restarts.`;
      if (await confirm(`Reset ${name}?`, prompt, 'Reset')) {
        send('previews', 'MODIFIED', {
          ...preview,
          metadata: {
            ...preview.metadata,
            annotations: { ...preview.metadata?.annotations, [resetAnnotation]: now() },
          },
          status: { ...preview.status, phase: 'Provisioning' },
        });
        toastManager.info(`Resetting ${name}`);
      }
    },
  };

  const Snapshot: ActionItem = {
    title: 'Take snapshot',
    icon: mdiCameraOutline,
    $if: () => can.snapshot,
    onAction: async () => {
      const snapshot = await modalManager.show(SnapshotModal, { preview });
      if (snapshot) {
        send('previewsnapshots', 'ADDED', {
          ...snapshot,
          metadata: { ...snapshot.metadata, creationTimestamp: now() },
          status: { phase: 'InProgress', appRef: preview.spec.appRef, parameters: preview.spec.parameters },
        });
        toastManager.info(`Taking snapshot ${snapshot.metadata?.name}`);
      }
    },
  };

  const Delete: ActionItem = {
    title: 'Delete',
    icon: mdiTrashCanOutline,
    color: 'danger',
    $if: () => can.delete,
    onAction: async () => {
      if (await confirm(`Delete ${name}?`, 'Its data is deleted with it.', 'Delete')) {
        send('previews', 'DELETED', preview);
        toastManager.success(`Deleted ${name}`);
        await goto(Route.app(namespace, preview.spec.appRef.name));
      }
    },
  };

  return { Edit, Reset, Snapshot, Delete };
};

/** getSnapshotActions returns the actions on a snapshot, apart from those the user may not take. */
export const getSnapshotActions = (snapshot: PreviewSnapshot) => {
  const namespace = snapshot.metadata?.namespace ?? '';
  const name = snapshot.metadata?.name ?? '';

  const Delete: ActionItem = {
    title: 'Delete',
    icon: mdiTrashCanOutline,
    color: 'danger',
    $if: () => canDeleteSnapshots(namespace) && snapshot.status?.phase !== 'Deleting',
    onAction: async () => {
      if (!(await confirm(`Delete ${name}?`, 'Previews can no longer start from it.', 'Delete'))) {
        return;
      }
      const user = list('previews', namespace).find((preview) => preview.spec.restoreFrom?.snapshotRef?.name === name);
      if (user) {
        send('previewsnapshots', 'MODIFIED', {
          ...snapshot,
          metadata: { ...snapshot.metadata, deletionTimestamp: now() },
          status: {
            ...snapshot.status,
            phase: 'Deleting',
            conditions: [
              {
                type: 'Ready',
                status: 'True',
                reason: 'InUse',
                message: `in use by Preview ${user.metadata?.name}`,
                lastTransitionTime: now(),
              },
            ],
          },
        });
        toastManager.warning(`${name} is deleted once Preview ${user.metadata?.name} no longer uses it`);
      } else {
        send('previewsnapshots', 'DELETED', snapshot);
        toastManager.success(`Deleted ${name}`);
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
