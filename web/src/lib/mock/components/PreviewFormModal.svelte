<script lang="ts">
  import {
    Field,
    FormModal,
    HelperText,
    Input,
    NumberInput,
    Select,
    Stack,
    Switch,
    type SelectOption,
  } from '@immich/ui';
  import type { Preview, PreviewApp } from '#lib/kube/kupra.gen.ts';
  import { channelLabel, imageTags } from '../data.ts';
  import { channel, channels, findPreview, snapshotsIn } from '../model.ts';

  type Props = {
    app: PreviewApp;
    /** preview is the preview to edit. Without it, the form creates one. */
    preview?: Preview;
    onClose: (preview?: Preview) => void;
  };

  const { app, preview, onClose }: Props = $props();

  const namespace = $derived(app.metadata?.namespace ?? '');
  const parameters = $derived(app.spec.parameters ?? []);

  const ready = $derived(snapshotsIn(namespace).filter((snapshot) => snapshot.status?.readyToUse));

  const restoreValue = (preview: Preview) => {
    const restoreFrom = preview.spec.restoreFrom;
    if (restoreFrom?.snapshotRef) {
      return `name:${restoreFrom.snapshotRef.name}`;
    }
    const selected = restoreFrom?.snapshotSelector?.matchLabels?.[channelLabel];
    return selected ? `channel:${selected}` : 'empty';
  };

  // New previews start from the channel of the newest snapshot that's ready to use.
  const defaultRestore = () => {
    const selected = ready[0] && channel(ready[0]);
    return selected ? `channel:${selected}` : 'empty';
  };

  const restoreOptions = $derived<SelectOption[]>([
    ...channels(namespace)
      .map((selected) => ({ selected, newest: ready.find((snapshot) => channel(snapshot) === selected) }))
      .filter(({ newest }) => newest !== undefined)
      .map(({ selected, newest }) => ({
        value: `channel:${selected}`,
        label: `Newest in ${selected} (${newest?.metadata?.name})`,
      })),
    ...ready.map((snapshot) => ({ value: `name:${snapshot.metadata?.name}`, label: snapshot.metadata?.name ?? '' })),
    { value: 'empty', label: 'Empty' },
  ]);

  const nameError = $derived.by(() => {
    if (preview || name === '') {
      return;
    }
    if (!/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(name) || name.length > 63) {
      return "must consist of lower case alphanumeric characters or '-', and start and end with one";
    }
    if (findPreview(namespace, name)) {
      return 'already exists';
    }
  });

  // The form starts from the preview being edited, or from the app's defaults.
  const initial = () => ({
    name: preview?.metadata?.name ?? '',
    values: {
      ...Object.fromEntries(parameters.flatMap(({ name, default: value }) => (value ? [[name, value]] : []))),
      ...preview?.spec.parameters,
    } as Record<string, string>,
    restore: preview ? restoreValue(preview) : defaultRestore(),
  });
  const start = initial();
  let name = $state(start.name);
  let values = $state(start.values);
  let restore = $state(start.restore);

  const valueOf = (name: string) => values[name] ?? '';

  const missing = $derived(parameters.some((parameter) => parameter.required && valueOf(parameter.name) === ''));

  const onSubmit = () => {
    const restoreFrom = restore.startsWith('channel:')
      ? { snapshotSelector: { matchLabels: { [channelLabel]: restore.slice('channel:'.length) } } }
      : restore.startsWith('name:')
        ? { snapshotRef: { name: restore.slice('name:'.length) } }
        : undefined;
    const spec = { ...preview?.spec, appRef: { name: app.metadata?.name ?? '' }, parameters: values, restoreFrom };
    onClose({ ...preview, metadata: { ...preview?.metadata, name, namespace }, spec });
  };
</script>

<FormModal
  title={preview ? `Edit ${preview.metadata?.name}` : `New ${app.metadata?.name} preview`}
  icon={false}
  size="medium"
  submitText={preview ? 'Save' : 'Create'}
  disabled={name === '' || nameError !== undefined || missing}
  onClose={() => onClose()}
  {onSubmit}
>
  <Stack gap={4}>
    <Field label="Name" required="indicator" invalid={nameError !== undefined} readOnly={preview !== undefined}>
      <Input bind:value={name} placeholder="pr-1234" autocomplete="off" />
      {#if nameError}
        <HelperText color="danger">{nameError}</HelperText>
      {/if}
    </Field>

    {#each parameters as parameter (parameter.name)}
      {@const required = parameter.required ? 'indicator' : false}
      {#if parameter.type === 'boolean'}
        <Field label={parameter.title} description={parameter.description}>
          <Switch
            checked={values[parameter.name] === 'true'}
            onCheckedChange={(checked) => (values[parameter.name] = String(checked))}
          />
        </Field>
      {:else if parameter.type === 'integer'}
        <Field label={parameter.title} description={parameter.description} {required}>
          <NumberInput
            bind:value={
              () => (valueOf(parameter.name) === '' ? undefined : Number(valueOf(parameter.name))),
              (value) => (values[parameter.name] = value === undefined ? '' : String(value))
            }
          />
        </Field>
      {:else if parameter.uiHint?.imageTag || parameter.enum}
        {@const options = parameter.enum ?? imageTags[parameter.uiHint?.imageTag?.repository ?? ''] ?? []}
        <Field label={parameter.title} description={parameter.description} {required}>
          <Select
            {options}
            bind:value={() => values[parameter.name] ?? '', (value) => (values[parameter.name] = value)}
          />
        </Field>
      {:else}
        <Field label={parameter.title} description={parameter.description} {required}>
          <Input bind:value={() => values[parameter.name] ?? '', (value) => (values[parameter.name] = value)} />
        </Field>
      {/if}
    {/each}

    <Field label="Start from">
      <Select options={restoreOptions} bind:value={restore} />
      <HelperText>A channel's newest snapshot is picked again on every reset.</HelperText>
    </Field>
  </Stack>
</FormModal>
