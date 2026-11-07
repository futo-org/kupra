<script lang="ts">
  import { Field, FormModal, HelperText, Input, Select, Stack } from '@immich/ui';
  import { DateTime } from 'luxon';
  import type { Preview, PreviewSnapshot } from '#lib/kube/kupra.gen.ts';
  import { channelLabel } from '../data.ts';
  import { channels, snapshotsIn } from '../model.ts';

  type Props = {
    preview: Preview;
    onClose: (snapshot?: PreviewSnapshot) => void;
  };

  const { preview, onClose }: Props = $props();

  const namespace = $derived(preview.metadata?.namespace ?? '');
  const initialName = () => `${preview.metadata?.name}-${DateTime.now().toFormat('yyyy-MM-dd-HHmm')}`;

  let name = $state(initialName());
  let channel = $state('manual');
  let mode = $state<'Offline' | 'Online'>('Offline');

  const channelOptions = $derived([...new Set(['manual', ...channels(namespace)])]);
  const exists = $derived(snapshotsIn(namespace).some((snapshot) => snapshot.metadata?.name === name));

  const onSubmit = () =>
    onClose({
      metadata: { name, namespace, labels: { [channelLabel]: channel } },
      spec: { source: { previewRef: { name: preview.metadata?.name ?? '' } }, mode },
    });
</script>

<FormModal
  title={`Snapshot ${preview.metadata?.name}`}
  icon={false}
  submitText="Take snapshot"
  disabled={name === '' || exists}
  onClose={() => onClose()}
  {onSubmit}
>
  <Stack gap={4}>
    <Field label="Name" required="indicator" invalid={exists}>
      <Input bind:value={name} autocomplete="off" />
      {#if exists}
        <HelperText color="danger">already exists</HelperText>
      {/if}
    </Field>
    <Field label="Channel">
      <Select options={channelOptions} bind:value={channel} />
      <HelperText>New previews can start from a channel's newest snapshot.</HelperText>
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
