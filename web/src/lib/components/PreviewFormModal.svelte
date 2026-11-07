<script lang="ts">
  import { Field, FormModal, HelperText, Input, Select, Stack, Switch, type SelectOption } from '@immich/ui';
  import { dnsLabelError } from '#lib/kube/names.ts';
  import {
    integerError,
    missingParameters,
    parameterDefaults,
    parameterInput,
    withParameter,
  } from '#lib/kube/preview.ts';
  import { apiVersion, type Preview, type PreviewApp } from '#lib/kube/types.ts';
  import { findPreview, restoreLabel, snapshotsOf } from '#lib/model.ts';

  type Props = {
    app: PreviewApp;
    /** preview is the preview to edit. Without it, the form creates one. */
    preview?: Preview;
    onClose: (preview?: Preview) => void;
  };

  const { app, preview, onClose }: Props = $props();

  const namespace = $derived(app.metadata?.namespace ?? '');
  const parameters = $derived(app.spec.parameters ?? []);

  // A preview starts from one of its app's snapshots that's ready to use, or empty.
  const ready = $derived(
    snapshotsOf(namespace, app.metadata?.name ?? '').filter((snapshot) => snapshot.status?.readyToUse),
  );

  const restoreValue = (preview: Preview) => {
    const restoreFrom = preview.spec.restoreFrom;
    if (restoreFrom?.snapshotRef) {
      return `name:${restoreFrom.snapshotRef.name}`;
    }
    return restoreFrom ? 'selector' : 'empty';
  };

  const restoreOptions = $derived<SelectOption[]>([
    { value: 'empty', label: 'Empty' },
    // A selector set some other way, such as by GitOps, is kept as it is.
    ...(preview?.spec.restoreFrom?.snapshotSelector ? [{ value: 'selector', label: restoreLabel(preview) }] : []),
    ...[
      ...new Set([
        ...ready.map((snapshot) => snapshot.metadata?.name ?? ''),
        ...(preview?.spec.restoreFrom?.snapshotRef ? [preview.spec.restoreFrom.snapshotRef.name] : []),
      ]),
    ].map((name) => ({ value: `name:${name}`, label: name })),
  ]);

  const nameError = $derived.by(() => {
    if (preview || name === '') {
      return;
    }
    const invalid = dnsLabelError(name);
    if (invalid) {
      return invalid;
    }
    if (findPreview(namespace, name)) {
      return 'already exists';
    }
  });

  // The form starts from the preview being edited, or from the app's defaults.
  const initial = () => ({
    name: preview?.metadata?.name ?? '',
    values: {
      ...parameterDefaults(parameters),
      ...preview?.spec.parameters,
    } as Record<string, string>,
    restore: preview ? restoreValue(preview) : 'empty',
  });
  const start = initial();
  let name = $state(start.name);
  let values = $state(start.values);
  let restore = $state(start.restore);

  const valueOf = (name: string) => (Object.hasOwn(values, name) ? values[name] : undefined) ?? '';

  const missing = $derived(missingParameters(parameters, values).length > 0);
  const invalid = $derived(
    parameters.some((parameter) => parameter.type === 'integer' && integerError(valueOf(parameter.name)) !== undefined),
  );

  const onSubmit = () => {
    const restoreFrom = restore.startsWith('name:')
      ? { snapshotRef: { name: restore.slice('name:'.length) } }
      : restore === 'selector'
        ? preview?.spec.restoreFrom
        : undefined;
    const spec = { ...preview?.spec, appRef: { name: app.metadata?.name ?? '' }, parameters: values, restoreFrom };
    onClose({ ...preview, apiVersion, kind: 'Preview', metadata: { ...preview?.metadata, name, namespace }, spec });
  };
</script>

<FormModal
  title={preview ? `Edit ${preview.metadata?.name}` : `New ${app.metadata?.name} preview`}
  icon={false}
  size="medium"
  submitText={preview ? 'Save' : 'Create'}
  disabled={name === '' || nameError !== undefined || missing || invalid}
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
      {@const input = parameterInput(parameter)}
      {#if input === 'enum'}
        <Field label={parameter.title} description={parameter.description} {required}>
          <Select
            options={parameter.enum ?? []}
            bind:value={() => valueOf(parameter.name), (value) => (values = withParameter(values, parameter, value))}
          />
        </Field>
      {:else if input === 'boolean'}
        <Field label={parameter.title} description={parameter.description} {required}>
          <Switch
            checked={valueOf(parameter.name) === 'true'}
            onCheckedChange={(checked) => (values = withParameter(values, parameter, String(checked)))}
          />
        </Field>
      {:else if input === 'integer'}
        <Field label={parameter.title} description={parameter.description} {required}>
          <!-- A text input keeps the value as typed: a JavaScript number can't hold every 64-bit integer. -->
          <Input
            inputmode="numeric"
            bind:value={() => valueOf(parameter.name), (value) => (values = withParameter(values, parameter, value))}
          />
          {#if integerError(valueOf(parameter.name))}
            <HelperText color="danger">{integerError(valueOf(parameter.name))}</HelperText>
          {/if}
        </Field>
      {:else}
        {@const repository = parameter.uiHint?.imageTag?.repository}
        <!-- Without a source of an image's tags, the imageTag hint only says which image the tag is for. -->
        <Field
          label={parameter.title}
          description={parameter.description ?? (repository && `Tag of ${repository}`)}
          {required}
        >
          <Input
            bind:value={() => valueOf(parameter.name), (value) => (values = withParameter(values, parameter, value))}
          />
        </Field>
      {/if}
    {/each}

    <Field label="Start from">
      <Select options={restoreOptions} bind:value={restore} />
      <HelperText>Resetting the preview restores this again.</HelperText>
    </Field>
  </Stack>
</FormModal>
