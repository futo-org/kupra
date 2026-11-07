<script lang="ts">
  import { Field, FormModal, HelperText, Input, Select, Stack } from '@immich/ui';
  import { DateTime } from 'luxon';
  import { dnsSubdomainError } from '#lib/kube/names.ts';
  import { apiVersion, type Preview, type PreviewSnapshot } from '#lib/kube/types.ts';
  import { store } from '#lib/watch.svelte.ts';

  type Props = {
    preview: Preview;
    onClose: (snapshot?: PreviewSnapshot) => void;
  };

  const { preview, onClose }: Props = $props();

  const namespace = $derived(preview.metadata?.namespace ?? '');
  const initialName = () => `${preview.metadata?.name}-${DateTime.now().toFormat('yyyy-MM-dd-HHmm')}`;

  let name = $state(initialName());
  let mode = $state<'Offline' | 'Online'>('Offline');

  const nameError = $derived(
    name === ''
      ? undefined
      : (dnsSubdomainError(name) ??
          (store.list('previewsnapshots', namespace).some((snapshot) => snapshot.metadata?.name === name)
            ? 'already exists'
            : undefined)),
  );

  const onSubmit = () =>
    onClose({
      apiVersion,
      kind: 'PreviewSnapshot',
      metadata: { name, namespace },
      spec: { source: { previewRef: { name: preview.metadata?.name ?? '' } }, mode },
    });
</script>

<FormModal
  title={`Snapshot ${preview.metadata?.name}`}
  icon={false}
  submitText="Take snapshot"
  disabled={name === '' || nameError !== undefined}
  onClose={() => onClose()}
  {onSubmit}
>
  <Stack gap={4}>
    <Field label="Name" required="indicator" invalid={nameError !== undefined}>
      <Input bind:value={name} autocomplete="off" />
      {#if nameError}
        <HelperText color="danger">{nameError}</HelperText>
      {/if}
    </Field>
    <Field label="Mode">
      <Select
        options={[
          { value: 'Offline', label: 'Offline' },
          { value: 'Online', label: 'Online' },
        ]}
        bind:value={mode}
      />
      <HelperText>
        {mode === 'Offline'
          ? 'Stops the preview while capturing, so its data is consistent.'
          : 'Captures while the preview runs. Data that spans volumes may not line up.'}
      </HelperText>
    </Field>
  </Stack>
</FormModal>
