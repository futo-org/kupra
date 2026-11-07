import type { Color } from './conditions.ts';
import type { Preview, PreviewApp } from './types.ts';

/** createdByAnnotation records who created a Preview. */
export const createdByAnnotation = 'kupra.futo.tech/created-by';

/** sourceAnnotation records where a Preview came from, such as a pull request's URL. Whatever creates the Preview sets it. */
export const sourceAnnotation = 'kupra.futo.tech/source';

/** Phase is a Preview's phase. */
export type Phase = NonNullable<NonNullable<Preview['status']>['phase']>;

/** phaseColor returns the color for a Preview's phase. */
export const phaseColor = (phase: Phase | undefined): Color => {
  switch (phase) {
    case 'Running': {
      return 'success';
    }
    case 'Failed': {
      return 'danger';
    }
    case 'Pending':
    case 'Provisioning':
    case 'Starting': {
      return 'info';
    }
    case 'Stopping':
    case 'Stopped':
    case 'Terminating':
    case undefined: {
      return 'secondary';
    }
  }
};

/** Owner is someone who owns a Preview. */
export type Owner = {
  kind: string;
  name: string;
  /** creator is set for whoever created the Preview. */
  creator: boolean;
};

/** owners returns a Preview's owners: whoever created it, then its spec.owners. */
export const owners = (preview: Preview): Owner[] => {
  const result: Owner[] = [];
  const createdBy = preview.metadata?.annotations?.[createdByAnnotation];
  if (createdBy) {
    result.push({ kind: 'User', name: createdBy, creator: true });
  }
  for (const { kind, name } of preview.spec.owners ?? []) {
    if (result.every((owner) => owner.kind !== kind || owner.name !== name)) {
      result.push({ kind, name, creator: false });
    }
  }
  return result;
};

/** ParameterRow is one of a Preview's parameters, as the dashboard shows it. */
export type ParameterRow = {
  name: string;
  /** title is the PreviewApp's title for the parameter, if it has one. */
  title?: string;
  value: string;
  /** isDefault is set when the Preview gives no value and the PreviewApp's default applies. */
  isDefault: boolean;
};

/**
 * parameterRows returns a Preview's parameters, in the PreviewApp's order, with the app's defaults for those the
 * Preview leaves out. Values for parameters the app doesn't declare come last.
 */
export const parameterRows = (preview: Preview, app: PreviewApp | undefined): ParameterRow[] => {
  const values = preview.spec.parameters ?? {};
  const declared = app?.spec.parameters ?? [];
  const rows: ParameterRow[] = [];
  for (const parameter of declared) {
    const value = values[parameter.name];
    if (value !== undefined) {
      rows.push({ name: parameter.name, title: parameter.title, value, isDefault: false });
    } else if (parameter.default !== undefined) {
      rows.push({ name: parameter.name, title: parameter.title, value: parameter.default, isDefault: true });
    }
  }
  const names = new Set(declared.map(({ name }) => name));
  for (const [name, value] of Object.entries(values).toSorted(([a], [b]) => a.localeCompare(b))) {
    if (!names.has(name)) {
      rows.push({ name, value, isDefault: false });
    }
  }
  return rows;
};

/** ownerLabel returns how the dashboard names an owner: users by name, and others by kind and name. */
export const ownerLabel = ({ kind, name }: Owner) => (kind === 'User' ? name : `${kind} ${name}`);

/** withParameter returns a Preview's parameter values with one set, or left out when it's empty, so the PreviewApp's default applies. */
export const withParameter = (values: Record<string, string>, name: string, value: string) => {
  const { [name]: _, ...rest } = values;
  return value === '' ? rest : { ...rest, [name]: value };
};
